/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ibm/cassandra-operator/controllers/nodectl"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/ibm/cassandra-operator/controllers/webhooks"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	nwv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/go-logr/zapr"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/logger"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var shutdown = false                //to track if the shutdown process has been started. Used for graceful shutdown
var waitGroup = &sync.WaitGroup{}   //waits until the reconcile loops finish. Used to gracefully shutdown the environment
var mgrStopCh = make(chan struct{}) //stops the manager by sending a value to the channel
var mockProberClient = &proberMock{}
var mockNodectlClient = &nodectlMock{}
var mockNodetoolClient = &nodetoolMock{}
var mockCQLClient = &cqlMock{}
var mockReaperClient = &reaperMock{}
var operatorConfig = config.Config{}
var ctx = context.Background()
var logr = zap.NewNop()
var reconcileInProgress = false
var testFinished = false
var enableOperatorLogs bool
var (
	caCrtBytes, _      = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZGakNDQXY2Z0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFkTVJzd0dRWURWUVFLRXhKRFlYTnoKWVc1a2NtRWdUM0JsY21GMGIzSXdIaGNOTWpJd01qRTNNVE14TURJM1doY05Nekl3TWpFMU1UTXhNREkzV2pBZApNUnN3R1FZRFZRUUtFeEpEWVhOellXNWtjbUVnVDNCbGNtRjBiM0l3Z2dJaU1BMEdDU3FHU0liM0RRRUJBUVVBCkE0SUNEd0F3Z2dJS0FvSUNBUURJYnNFQjFNOVFIa0h6bld1S0VFUE40NHV4WEg4cG5vRDFsYWQyeFRWNUd3eHIKWnhtQmhIY1Y1Tk1ZZFJUbWEvYksyclZHbi9vQkhqRGY3VWVvOUlqUjVlTXFYc2tqYTZZSTJ3UGxuK3ZtR29hagpsQ0xOUmEwb01Xa1JFM0RJSStTbUthNk9LTDB6ek9oK3dnN2dBYVhqa0VxbXRVVStqcHBJY1VPbXgvcmM3Qlk2Cmxvc3dmWXVmN1MvTE9CY2N1WFBZd214SWt2MEkrSFhDMjdmcVQ0ZmlzV2xCYUtxV2VINVdBcWpsd2FHMVUvMW4KR1NmTHYrcW5OZERkWkwxeERlcDljWW40UitLcTFvQjE0b3VZaklXUVdSUk9UNHFuWHAxQ25kanNXZDVoUitWeApIL1Vac2RiOE9zTm9KOUVEYXNVMmtsYnljVGJjNE1UbSt0RUdOYzdNNVNMT0dHY3BpMzhzV2dEeTNUKzI0N1NUCkkycDl6WWdOdWYxcHZmNW8zbU1KWW0yVndiUC9qUHk2bmlRaXFJQ1FyUzU5d3hsaTBiTVFHN2E4ZWIxT2dPd0UKdHlGT1RVRm9kemdLVDQzbzVJanF4blE0TEZWZlZXYytHakgza3JQOEk0RGJSRW56bERUMDhXWE4xYVFMZmwvSgpBb2tzbFZzc2l1d1lza1h5Z0M1SjVONkdrUkhWa1pUZVNZY3Rna0x4MDFCeHNJSFJIV2E2M29CMnZFc2Nnanc0CnQ4TDZncHFBUGZlTlhIVFlORzJYeTdsZGlYNUJsR3QvT2FpU1hRZE1ZRHpuT3ZrOXAwRWt6VGJpdkRveHJkZlgKeWxnbHJESjBXcWJPcDUrdVhSVGFXaGt2TUc1cnJLMDk2NXU1c3BaanU5ODNZWk56eW9aTk1YK2FEYkswc3dJRApBUUFCbzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSEF3RUdDQ3NHCkFRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRktzb2RIQW9lczFDVHVDUmRYTDEKa0FPdkZKdUVNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUNBUUF3a2RVdmZjTytOMWxIcFVXOGh3WE5FZkZRVDl5MQp2dnd4NGc1ODNNckRhNDVINmwvcU5waTVzbjBHbkF6aHNHL2swNHdleURwTEJUR1Fha3BQczVsRHA1b1NpL0l2CkVwWXE5bmhJcXZXbTlzYVpQUXQrVUpxTjcxK1VUUFdFOGZlQXFJSVZJQlMrdTVrNVZRQnVud2JIakJLU050cVAKVC9iYmdldjdxZG5ycEd2YWtpdlFxU2hmQUlyRGR2TGNJSlNNQnhIK0VvT2M1Y2xsUVpRRlR0Q29OYTR0THpPTwpVR2czWGVGRmYyMHIyTXE0U2pYNURzMHBIcTYwSC9DMGJZYmdxQjBFcHBNSXRwdTBZbndGUWlzdGY2OE5LV2xqClAxVG80a0d0bVBkdmo5NU9uVk5PUlFSYlF2NktwcU55R0hSdEM0MWpUbVV5WmVVK1k0Z1c4b2lXVHB4eDZEZHQKN2lTOU9FT0g2SXVocE1WYXdEdjFYdThYQll4NHFVRjRwNnVqVWRuRUZTVW9VQi9vWXVXVkQ3UFAzU3E5SHVVNQpXQTgybzF6UmY1OXF0Z2NsV3ZNTEJQWWtpR29CekhNVFQ0NENBcTRXRkZ0bTA5Y0JPdC81VHRYZ2xhSkhvRmRDCmpnc0tlWjhUYm15Q1VnWHZ4VDlpWkNnYTUzTnhxMGFudGRDUkNQbisrTE5lcFZBRkFuYW54eWdBT0V6cW5SdXgKQm1XNDdvbTNCdGdrMnFiS3FjMnI4anZtN3A3ajRoZFJPTW5GRHV6c1RPdVNnRlFBRWtWenMxYjEwdWFGbldWNApISURySXN2VzlQZjBBR0J3S1B1dVArUVNoME9QU25JUEZrNWM3U1gwSmlBelp6VHJGWXUxYUhaTzlXMHRPTFk5Ck1uaTBtYWVucDRyMXB3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=")
	caKeyBytes, _      = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlKSndJQkFBS0NBZ0VBeUc3QkFkVFBVQjVCODUxcmloQkR6ZU9Mc1Z4L0taNkE5WlduZHNVMWVSc01hMmNaCmdZUjNGZVRUR0hVVTVtdjJ5dHExUnAvNkFSNHczKzFIcVBTSTBlWGpLbDdKSTJ1bUNOc0Q1Wi9yNWhxR281UWkKelVXdEtERnBFUk53eUNQa3BpbXVqaWk5TTh6b2ZzSU80QUdsNDVCS3ByVkZQbzZhU0hGRHBzZjYzT3dXT3BhTApNSDJMbiswdnl6Z1hITGx6Mk1Kc1NKTDlDUGgxd3R1MzZrK0g0ckZwUVdpcWxuaCtWZ0tvNWNHaHRWUDlaeGtuCnk3L3FwelhRM1dTOWNRM3FmWEdKK0VmaXF0YUFkZUtMbUl5RmtGa1VUaytLcDE2ZFFwM1k3Rm5lWVVmbGNSLzEKR2JIVy9EckRhQ2ZSQTJyRk5wSlc4bkUyM09ERTV2clJCalhPek9VaXpoaG5LWXQvTEZvQTh0MC90dU8wa3lOcQpmYzJJRGJuOWFiMythTjVqQ1dKdGxjR3ovNHo4dXA0a0lxaUFrSzB1ZmNNWll0R3pFQnUydkhtOVRvRHNCTGNoClRrMUJhSGM0Q2srTjZPU0k2c1owT0N4VlgxVm5QaG94OTVLei9DT0EyMFJKODVRMDlQRmx6ZFdrQzM1ZnlRS0oKTEpWYkxJcnNHTEpGOG9BdVNlVGVocEVSMVpHVTNrbUhMWUpDOGROUWNiQ0IwUjFtdXQ2QWRyeExISUk4T0xmQworb0thZ0QzM2pWeDAyRFJ0bDh1NVhZbCtRWlJyZnptb2tsMEhUR0E4NXpyNVBhZEJKTTAyNHJ3Nk1hM1gxOHBZCkphd3lkRnFtenFlZnJsMFUybG9aTHpCdWE2eXRQZXVidWJLV1k3dmZOMkdUYzhxR1RURi9tZzJ5dExNQ0F3RUEKQVFLQ0FnQmUwUjRYR3JCVUMyeFJyY2ZBMFg3eCtGSU9QbDZkdHJEMC9LM3pIc24wRjVxaGVHMTFlcy9IR0svUQpJeHNYQWo0R3FyNFV3ZnRINmh0ZTQyWUNCR1J4UDFwZW9lWnZEaTdHZzYxdFJHRVpRclVzenhoRG1WR1g4UC91Ckp3ODBidDVzeU0wZHpTSHNUbVF5Q3VWMGpQTUlHeXRsZjkxWkFhYjAzRGdQdnd6cTAvaVVFTUdaMTlwa2RwVWsKZ3MxVU5sc2FVS2RmRWNJSUsxbXlLN1R1Y0Y4dEc1WmFiZ1E5R0pWVFpRQzNhQWx5dVYvb0ZOdGhwTkdCOXBCdgpHNGdPNG5GcWxIcWJiSTVMR1J3K0tFUzNqc3BraGU5NE9HSzBXS3IyWjZ2RjY2R3F6Wk02Rjg4Z0w2UXZRREZPCnBYOVlLWHRRSEp4ckpxbTRoZEJBSUtrZlVmY1kyU1JDZmdrbU5JdU9Hamdob3JqeTNWandkSVBZNWRJemtNVEIKRUZIeUtSTnRKNEdjazRKRG9PTFZCWnJyWkQ2QlNXdDN6UERRSmVCd2htblBYamdZQVE0YjNjK2F2NFNWQlJCeQo2WjRkZEsyd1BqQ25OM1hJNzZWTjhCTGhkaE1zYmxaWmFSYjVkRE5oZXpmMkFYNW1YenNZV3pjOGpTQU1YM0h2Cm9GV1RlOEtkYmg0M1lNY0RDcWx3RzYrVWdUdFVjKzlMRm1EYUR2NVFiaWp0ZE5aQk5OUU9KZmd3eGtXZ0RpaEMKQzdScG1mZWFKVkQyS3VOdExBdENPRUxvNFNOOERWdk1yTHBESW9JWnRIZjFqdHdmZVBzRnN1Vk45M2dTZ1pDYwoxNFhtaFZlSkZyanNiVFdQVUUrbktFWWdqdGVFd2hGaWIvNmZSYmdFVnZUczc2OFMrUUtDQVFFQXltM0tCbVdTCjh2SG5VUG0yTUhHemVBL2FqVVAvN3dMRDlGU0NiZUE1NUFzREhDUmVGb0QvV3RJWnRDYktXaUZjZGpQU3dhUVcKVEpwbDV4clY1Tm0ybGJNdmQzYTFwcHRDd2lGYWhnbml5Rm40VTdPb2hib0crdnlRMkJaSHhHbzF4d1hrckRwTgpvbXR1VUlpMU9ZMXY0eGg4RFBIRlJlZ2x5T2tWRTFvM2tGamEwT1hvYkw1cUFzWjZ4ZU1ielp4eWNRWFZZZWxwCklDRW5CZFl2dFk5T0xORWhyaDBkNFpjSUpWcGVrajk1QUppbkhOQnRzOHdCc2RhK0xVVXBpTDgrQXllNWc1RG4KSEdldGlON2lEL0xLRUcweGtBWGxpMWRPak1xQ3V3aGt0dGpLN280ZFdFY3hGT3daaDFHaHVwL3RCNjBJbldRNwpMK0Rmck5QczhHaHN6UUtDQVFFQS9YbTVOejI5am1PZFcxbnRMTjYrK1R0OStRTVkvakhZWm0wVXdJaEFtMk9DCktzaHgyVE9tNXBGRVkwTzYydkhHT0dqaVY2OEtqaC84V1psSTh6Mng0djh3anVZemJKVEpQTGYxb29RMGRiYzUKd2syMUhQTXV3ZjV0YjBqL2lkQk9hWGlIaWRTbllUcUhlVkwvOFFjSENXQ0ZqM0d6NnhQS2p1NEdDYlhhWXRDNQp4b00rc0I0K3JxckNzZGZHU3lwSlFXRk5KYmJRYVhqUGM1NDNmUEp2aWlGck9vTDMvM3RMdldzTkhmZG1DVTRoCmtTSWhoUnZWcnp2Y3lkdkVKM2hzODY0UjRIaXlZOHlHWmRpa2NOaUQ0dzNsNnJKS3VXcTk0WllPa2xNZGgxNVYKSkc1N1dLbENqY25Jbk9RQWp6dzNVeFRHUGlUT2RsNmhLTTVzZHBlbmZ3S0NBUUJDVE8xRFpSZFpQUVBQVU1wcwpXWUUzakxHL1hRdEJaRDE4RkFYWUtQMnRCREpUa0ZIRXV5Rm54TEtvZjUvOUh6b2llTnpKa1kzQUx6MjdFTjRICm80c2F3dUtFRlR4dndpQitadUE0VUpxWGxtZ3dPZ0t6TWZmQlV1RzU5S295MmJxZFlmL0FyU1BxVTVlQkJ4V2MKTVFmNWNIYUk0dE1ERDRMNHArYkFQT2MvL3VwRVMxang3UGZaeXRwQllCNG1ITnlheWhkV2gxVm9NWk9QWk5TaAplYnRZRUhNZ2pPYlJrVjhZcE4yZXR1MVIxYTIrVVVIdEJwOXplT3MyOXBVZzljcEF6RTBGbTNzbW9ZcUQ3c1JLCkJ2SkpxUW4zcXdiQXVhcS9rRUI3TThlUTM3YXZwWnBVNUpSZHp1cVptSklKQndKaVpqa1JHOWdLMlhOSkx1eEcKM1Z6dEFvSUJBRCtTcU81KzhLem10USsxVlRQOHhkOFNtYnk3bHlnaDdrbDZNRXM5b1I2WDdZeTNhejV6b3ZlUApGWnpqM3RpTTdRODIxeFh3MCsvamU5SXBETS9jK0dHYmFWMWR4U1lGaHhkUWVDNERoSGpGdEpuVURZbXVRRnJ0CmFoc1FMdThzckkzdGFla2F5Y1FyL3RCaURja3czd1h1REhGMnJnNVdqMllic3EzNnkwUWZYNGkzWUNDaDVVeS8KalVjM2ZBZGNHclZvSndZL2ZMUUhWZGlFcFJ3VVhmOUI5SGZmWXozVGVhS1BWK0hkSzkxSG1FbWpTczdzdFVKVwovRUF3ZTFqKzdpeUx5dllHcjQ4eU83OE5mK2pCbFFwOGNON1ZTc0tJVUFsbExsQnF3aXd5YjU1TWkya29Rb1gzClJ2WjZoTjFuMStSaGdIc1RsaWpBQVNHUDdFb3VMUmNDZ2dFQWJFVWQ0a1lWSmlsOGlOeWp3eHJsQ3BaZ0hRcTYKcnNLSW9XZXhCNFFDY256c0ZZMzI0YjVVTUNqc3p1aXpnY3RJWEl3Z2I5R1hmNmVhVXNqNU9BYVV1Rlk0SVZNKwpvSGRDODZSbHdYeEs2c00xU0tCaEkrZnI4YU1OVU16RkF6QmN4cWhoTG4rckNjYU5RSWNHU09JTHBPNm8vMlEvClFSclVKQWpEZVFWbVdGdnZKa00rV3kwQVBLOEoxQnI4UG5vNnZqam0rc2Y5dWZhVEUzWGhWRnZUTm9DK055TDkKcElpN0RPOGFRTGJRcEVjNThtcmIrTDZFcGhzYkdRWjVxMGJ3WENRcHh0Uy92MjE4NklxVER0aSsxOEpoTndmaAp5WFViNFRGOGFjMzUwSGYxZ21vbHBBUFgwNDVkUjZzdUxlUlR5RHE1aHhyVWs3RjBLMHNrZ0h1Ti93PT0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K")
	tlsCrtBytes, _     = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZQekNDQXllZ0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFkTVJzd0dRWURWUVFLRXhKRFlYTnoKWVc1a2NtRWdUM0JsY21GMGIzSXdIaGNOTWpJd01qRTNNVE15TlRFeldoY05Nekl3TWpFMU1UTXhNREkzV2pBeApNUnN3R1FZRFZRUUtFeEpEWVhOellXNWtjbUVnVDNCbGNtRjBiM0l4RWpBUUJnTlZCQU1UQ1d4dlkyRnNhRzl6CmREQ0NBaUl3RFFZSktvWklodmNOQVFFQkJRQURnZ0lQQURDQ0Fnb0NnZ0lCQU1nd2JuUkIzVFNUT2Q1V0JORUMKSVludlZGSGtuRDkwZ1NpZjFOK0VaN3hlWGkyZEd2cWxOazN2RVhIS1pvZ2RQWGd6RHR5aXdHaHJLL1BJdWhYdQpsSXdZWnNFbFNObzNXWnBiSExOLzRvZVVVNXJLbHhkV3BvaTN3MlVCdGIrVkx1L2FTbnpPdk82UHV5ZGxJcnowCjM4L2FQbWRteGhLRWhhbzJCdnBQNkE1MUc4bmY3OHhRaGVHQUJDcjdZQXUycE1kSHdSdEZoMEtGRDJIaFhtbTYKb2lhT3lxTkN0WUUzNU11OWNuRFp6Y3I1ZnBHakQ1TXJnSm00QVB5aWc4Mi9uczlOWlRsRUEvOVBac2x5NjlUcgp2ZC9iaXRzeVBGeVZRRTBoZ2JjTmU3OHNLMStIZEI4YkJGaW8wZnoxVVBhUnVib3RHcjRab3VCK2QySHUyQndRCjFBTnBQQytEbWY1OFZrekZ5SDEwSk9rNnhjS1BiaExpK3dOWk9DYnFhcnVQTWh4c0grNTJkNWRwNnk5d2FqS0QKTWVnNjFRWlU0eGVmc1JQa0FoR0p2RS9VRXlIY0tzb1g5NlozNVRyb2ZOSy9saEUrKzljamxZM0UzNkhFRy92egpURE5UNlNOYTNocFA4QTFsUW0vNm4xcVdtNXhvSWM5cGJTdHZTUjN5UkRURnlmZTM0TVJIZnl3YUIxOU1FZFNBCnZPSGIxalVqelRVVmsyLzd2RVRnUmwyYjFFSVFHeVBBdzlsUXF0ZnAvMmlyMHJWaE5sSUdWcWNSQkUrd0lLZnUKdHlYcE9WQXRuUkFVb1BKR2paRVQ0UnJRS0RLeTJKWnFJY0RQL2J5MDBqMkJJTG9NczFLbitsaW5yRlovd1VtVwpUM21ocUlWT2UrTzROOTBaeC9XcVcycG5BZ01CQUFHamRqQjBNQTRHQTFVZER3RUIvd1FFQXdJSGdEQWRCZ05WCkhTVUVGakFVQmdnckJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFmQmdOVkhTTUUKR0RBV2dCU3JLSFJ3S0hyTlFrN2drWFZ5OVpBRHJ4U2JoREFVQmdOVkhSRUVEVEFMZ2dsc2IyTmhiR2h2YzNRdwpEUVlKS29aSWh2Y05BUUVMQlFBRGdnSUJBQkI2RGZhL2NGNTRWb25TbFRJdm0yV3lyc3k4WXllRG5SemZtOFNHCktqNWVKMmxEb3JpaVJVUDZuRE5LM3dRWjdzRXJMZSt0L3NpUmVIWFpkbVB0dHFpSVlxWStub1Fjd1AwYTJXMXkKUlJIbUY5SVM5eGZDQzlHeEtuNlBSNWNYYTBVM2lvbStRRUxVWVYxbXlaRGdMdXZENjBLVlphTDFJWlJBK2Z6bQoxWDh4Q0RTN0lQNXRibFBuZVJKb0t2UlJpRm1ERzdtbXB0SFUxcEs3Zm1yWlZCZzFKSXdpNWVVRVJHNzRoU205CkFhVkpyeEY3SnUxMFplN2dNSG5EeUJLSUt1U1g0NXFhaUJWQlRsSGVJSk1SQStQTk5NT01JaWRvanhHRi9pVSsKZjljRzlhSnNDRHBNalJYcG1mU1FSYk5wdzlPM2pDR0NyN0xwS2EwL2xJUDBIWmp1bDcwTzFwK0Z5UTZGaXZKdwpOTWdiSHdWV25PZXU0UUxVb1RQRGIyVGQxNU5qaS9CaG9vVy9qdGRHZDF3dmxjWlcxVW91ekI1T2VOOThyT0NSCnQ3UkZHR3U2NnFtS250ODZWajhra0dwOVdqeW55Z0p0Z0tFTDVYZWF5OXRHS0dJTEhzQ1FvRmdSVUc5WkZHeUcKWEsvRU1MeWtEb1QrZnhDUWZoTU1SSmVOMVdpc3BjdnBqMnl5RytIOWxraXJwTm42WFhvS0tSV200ZjVDS0pOSQp1TTNSU2lvS1pReDk2WVUvZFpRUDVtVk9pMjkyRm0raXFvMkxPQmt6dlNDVVVKZWdjQXU2dHdmL2ZOUnpFaThGCnRoaXZiNU55cUprcGoyS2djWDc4Y3drVm5ndHRzMEdoUVNhdDc0bHZqQTltc0k5MkZuSmZLQXcrK1F0ZHZiRHkKR3JCQQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==")
	tlsKeyBytes, _     = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlKS1FJQkFBS0NBZ0VBeURCdWRFSGROSk01M2xZRTBRSWhpZTlVVWVTY1AzU0JLSi9VMzRSbnZGNWVMWjBhCitxVTJUZThSY2NwbWlCMDllRE1PM0tMQWFHc3I4OGk2RmU2VWpCaG13U1ZJMmpkWm1sc2NzMy9paDVSVG1zcVgKRjFhbWlMZkRaUUcxdjVVdTc5cEtmTTY4N28rN0oyVWl2UFRmejlvK1oyYkdFb1NGcWpZRytrL29EblVieWQvdgp6RkNGNFlBRUt2dGdDN2FreDBmQkcwV0hRb1VQWWVGZWFicWlKbzdLbzBLMWdUZmt5NzF5Y05uTnl2bCtrYU1QCmt5dUFtYmdBL0tLRHpiK2V6MDFsT1VRRC8wOW15WExyMU91OTM5dUsyekk4WEpWQVRTR0J0dzE3dnl3clg0ZDAKSHhzRVdLalIvUFZROXBHNXVpMGF2aG1pNEg1M1llN1lIQkRVQTJrOEw0T1ovbnhXVE1YSWZYUWs2VHJGd285dQpFdUw3QTFrNEp1cHF1NDh5SEd3ZjduWjNsMm5yTDNCcU1vTXg2RHJWQmxUakY1K3hFK1FDRVltOFQ5UVRJZHdxCnloZjNwbmZsT3VoODByK1dFVDc3MXlPVmpjVGZvY1FiKy9OTU0xUHBJMXJlR2svd0RXVkNiL3FmV3BhYm5HZ2gKejJsdEsyOUpIZkpFTk1YSjk3Zmd4RWQvTEJvSFgwd1IxSUM4NGR2V05TUE5OUldUYi91OFJPQkdYWnZVUWhBYgpJOEREMlZDcTErbi9hS3ZTdFdFMlVnWldweEVFVDdBZ3ArNjNKZWs1VUMyZEVCU2c4a2FOa1JQaEd0QW9NckxZCmxtb2h3TS85dkxUU1BZRWd1Z3l6VXFmNldLZXNWbi9CU1paUGVhR29oVTU3NDdnMzNSbkg5YXBiYW1jQ0F3RUEKQVFLQ0FnQlhML2k3TzRCYkNKQVlqSUEyZzJZV0RCMCtPWUh2aFE5SG9TejJXRlZSemd2WWMrY3ZLTXRZNy9rNgpCUHZZNWx0Q1FZS0VoNEdqT0tJQTMvaGoydS9wZ0Nzb2pkT0g3cmxncDdQOVhZSk1xRVl0VVhTeW5uT1RjZTF3CnpXalowdUNKYUJvdTkxK0R5eWVveGJ4MmJYUVlzNldnSlR1NUR1eWtNRG1qeFM5OU5IWHcyMDQyUHcvQUlhQXYKRkNKRmw3dDJhNExvSE1VSExLcUJaOFZWNlFuOEh3NlVRbGFJdTg0ekNnU1FyOXVZUGtkb3FJcjgvdUpZa0xJbApWYVp2OHJmNDgyMVZucERiSGpyWEcrMG9ZNi9qa2x1TWtmbUdIUjdQT1B1a3ZRT2JSR0p6amF2K2JDYmRjbEZhCmlZRkVrajFBbjZwWkJ4TlFFT1J2eTdWbHE5S2FEMXdzUEExck1MS2d0WHMraW5kNjlSb0g0Z0lHMWVyNDVyWEgKeFgyUzhlUEMxVnZPczVyMTFnbjlDTHZGM1g5TEkwRzhyUTdmMnhHT0JmUGZSRFNtQndia1NGRFQ4NWtKcmU4YgpxU0dWS3pnVmZLQitkWCtGc1pyMnowVzg0WkorNzFINXJNVmwzRFhUZDVvWm44MHMwTWJ2Vm0xQTR5ZThoS1haClVyWUlKaGk5aFBQTGRFdjkva215M0lZQ1RPeVA3M2lxQWtxWFROQk1sMU5nMXJEVnFnOEJMbWlQQnhsYkdEWkwKMUJiVEp6SGxEdUh0VER0aEgrTlFFamJtL2J2YlVWaWg5SnIyYklKd1RtZHJDOURubGlZckhPMVNERk9NelpXMQplZ3RoZnU2N3QxaTFEZzB5RXZ3QTRjd0l6N1RnSStZMWhWOFdnU1pMekxzazdVbTVJUUtDQVFFQXl3MW15TFdaCkExNHZKbDNDV2JGUGd4T3hwV1daTUlsTUUySDhBNElBY2xMcmZYYUVDaEZFYUZLUEhSNVc3M01GWkk1ZGI4ZCsKVDNnNHRWSFgvRm1kRHFWbkZHVEVjdlF6TjFFNDV2S2M5akNDYVNjYmppckxJMi93dGJmbjUxK20rZ2FZSHUyQgoxMWVQS0NjVlFMU1YrallSSWFvbzhwUXhMNTJBZzB3TFMwOFJWcDltSjJYZVJNRmw0Z04yMWRHODFBSHo0cE50Cks0TExjREh1WFVxaWhVb1pvSDNKdXVnQWw1OWpGaDI4U0d2N1Y5dzJybldLUjdEaWwrajZqUXJlTUJsVys4V1IKeXZ0TW5ZQUk2YXJkRkg2TEtRbzkyaDFTbjlxUFczQnU0WGZoS1cyTS8vS0Jmc0VmZXJSYWNPY0dNM2ZYQ21uUwpNd1BjaXZoRDdzdHFOd0tDQVFFQS9HUG0xakphSW9CZU1xL2oyZ3VuNGh3V2ZoVjluZkZtTGJzaGhKL2hvNTFZCkFrRE5PSzNQdzVQaGpNVmF0OXBscE5LT3dKdTZ6Y2lKbXY5L3lZZ3JIRFFsZHI5TlN5TFA0akRzUEgyTllRWG8KTWZ5aWZSbzZBWDhkVE05S25GampaN2RBUFA5Y20yWGR6c1pvWG0xSFBCbDhqZFByY3hXQXRSYVdiSGtMOWVjRwpGQzY4VHFjTDJ1MHdocjFwK1Q3Ukp5U3FmTkdlbDBJdTkwSG5SMGUyWE1UeWRSSHhtY3c0a1pHcXZ0RVBqcmlECkZmK1FYQUlJdjFrNGJ1M2xwWmlUTWQrOE94U2JkK1JlVXF6MW5wb2YyMm8zQTJsck91V1lGdksxWWc1b3dzTUsKSlBkVGgyclk2RHYyblp6WUZKTitnQWEyRzhiV2xWbFhHNVErQ2VRcFVRS0NBUUF6QmNjU0lDKzIzTy9VSURNNQovb2dRVTk2THhSL2RSbUxMYkErN2dldEN6dnRGcDRZK1VXQnpKbXUzMFd2ZTkzUWZkSGFlSSs3bFlUdytzN1ltClB3NXNJN3ZuTC9yOU44ZTIycjRGcW1rdW83bmhXbVplTHZxa2lQaGZjVHJndFBSc1YwUGFZYXdDeEluQWVUR3AKOUxiTEROTDVOcHpiZDhsMFFZdU5jb3BUL2laSk1meWxZYitjWDB5T29uZzErbUlNbEFFSXFpc0FoL2srMUEwbApmQitpaHFBeDUrbk5BWWRFa2xSL3RqRWRzYVNTeTA4aVAzN2p6TnJqZVRPY1JXTFhpMGFFTlgwUkdGeUVKeXdXCkswMHFYVEh5dWhRWXZzMklSWTlJVnRSRjY2MjBqMGFpK2ZqNE5PY0lHbEd2bFIzRnlSTk1GcE12Sm9WQWxtbmIKazFtNUFvSUJBUURvL3Z6NkhvK3hMQnBKNXJDTStaZXBtRTUzWlRXTEduQStwWE5pOFFvbnRqaXFFOUtna1d5cwoyNWNzRW9FV0cwc2NacmtjUEdldVU0UWREK09UVTk2Z2NjOW9HV0NzalYybUxZM1lwZnRmWjBtYzQrSEpaZTNJCnRlV0JwQmYzeitOWE93ZlZoOVNoTE9BZ1pHK3pSR1N2cWNPZlZ0VVViL1JhdUpoeldTZmVnY0ROM3ZzK0RONVgKNUFHWlVjRkVOR1ozSVZVMlYzbDFuOGFsd2pSVlRIR2dvTHhIc1NIOTNsY1dvNGdpRWZPdnlNeFRBWlB1TEg2UQp6emZXeUUzcG1ybkFJQkN2NWUxRU9CR1BkMVprYlZvZGY2ZDd3ZFVaRVIxZmlmNUNvSlM3djd4Y3RlcHBxQXpJCkQ1c3lrN215L1JxWjdCZ3YrbGJjbHhaZ1FuRW5SbmVSQW9JQkFRQ2lIUHg1NFA0aGd4S1hjYTBXVTh4cHhKeEIKQXF6eG5KTUVNdUNSOFdQZjVaYlc0RHFpanBHU0M4UzhML2dXS2VYaVpLZFhBUUx1ZW9wQmhIcGZpZnZXZjRFVwp5WnhZbm00V015U2VpS093V1YwT3Z6MDRNUkJxek5FdFIvMGNBOGFJa3MxNkF1MWlwNDhySWJKZkxsMGpIMXJTCm1hUmdBWEQ1bzQrblp5Rk5MZS84RmtYMEJjQTBIMUF0d1BOZ3k1YVhNSlh2SWs2U0IzaFpRS1h6Z3lObUUzSnkKWlVMTHNGMGpFWVFDUU9ZZk55bGF1MnpGelhvMU1oQk9leGJ6YTVGVVhGcDFkSEZNQW9ESnh6SGgzRkw0K0JZdgpKUklxb3g4S0hXYlZqMzNSc1NpbTVWVlJPVUFmai9iZXJKeGErV2dqbnorYzVOL1NHRlRIVEl1MkJiMEQKLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K")
	keystoreBytes, _   = base64.StdEncoding.DecodeString("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")
	truststoreBytes, _ = base64.StdEncoding.DecodeString("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")
	keystorePass       = "cassandra"
)

var (
	cassandraObjectMeta = metav1.ObjectMeta{
		Namespace: "default",
		Name:      "test-cassandra-cluster",
	}

	reaperDeploymentLabels = map[string]string{
		v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentReaper,
		v1alpha1.CassandraClusterInstance:  cassandraObjectMeta.Name,
	}
)

const (
	longTimeout   = time.Second * 15
	mediumTimeout = time.Second * 5
	shortTimeout  = time.Second * 1

	mediumRetry = time.Second * 2
	shortRetry  = time.Millisecond * 300
)

func init() {
	flag.BoolVar(&enableOperatorLogs, "enableOperatorLogs", false, "set to true to print operator logs during tests")
}

func createValidatingWebhookConf(namespace string, clusterRole *rbac.ClusterRole) *admissionv1.ValidatingWebhookConfiguration {
	conf := webhooks.CreateValidatingWebhookConf(namespace, clusterRole, []byte{})
	return &conf
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	var err error

	if enableOperatorLogs {
		logr = logger.NewLogger("console", zap.DebugLevel).Desugar()
	}

	ctrl.SetLogger(zapr.NewLogger(logr))
	logf.SetLogger(zapr.NewLogger(logr))

	clusterRole := &rbac.ClusterRole{}
	clusterRole.Name = "cassandra-operator"
	clusterRole.UID = "1"
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			ValidatingWebhooks: []*admissionv1.ValidatingWebhookConfiguration{
				createValidatingWebhookConf(operatorConfig.Namespace, clusterRole),
			},
		},
		ErrorIfCRDPathMissing: false,
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	operatorConfig = config.Config{
		Namespace:             "default",
		RetryDelay:            time.Second * 1,
		DefaultCassandraImage: "cassandra/image",
		DefaultProberImage:    "prober/image",
		DefaultJolokiaImage:   "jolokia/image",
		DefaultReaperImage:    "reaper/image",
	}
	sch := scheme.Scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: sch})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	createOperatorConfigMaps()

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&v1alpha1.CassandraCluster{}).SetupWebhookWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	cassandraCtrl := &controllers.CassandraClusterReconciler{
		Log:    logr.Sugar(),
		Scheme: sch,
		Client: k8sClient,
		Cfg:    operatorConfig,
		Events: events.NewEventRecorder(&record.FakeRecorder{}),
		ProberClient: func(url *url.URL, auth prober.Auth) prober.ProberClient {
			return mockProberClient
		},
		CqlClient: func(clusterConfig *gocql.ClusterConfig) (cql.CqlClient, error) {
			authenticator := clusterConfig.Authenticator.(*gocql.PasswordAuthenticator)
			roles := mockCQLClient.cassandraRoles
			for _, role := range roles {
				if role.Role == authenticator.Username {
					if role.Password != authenticator.Password {
						return nil, errors.New("password is incorrect")
					}
					if !role.Login {
						return nil, errors.New("user in not allowed to log in")
					}
					return mockCQLClient, nil
				}
			}

			return nil, errors.New("user not found")
		},
		ReaperClient: func(url *url.URL, clusterName string, defaultRepairThreadCount int32) reaper.ReaperClient {
			mockReaperClient.clusterName = clusterName
			return mockReaperClient
		},
		NodectlClient: func(jolokiaAddr, jmxUser, jmxPassword string, logr *zap.SugaredLogger) nodectl.Nodectl {
			return mockNodectlClient
		},
	}

	testReconciler := SetupTestReconcile(cassandraCtrl)
	err = controllers.SetupCassandraReconciler(testReconciler, mgr, zap.NewNop().Sugar(), make(chan event.GenericEvent))
	Expect(err).ToNot(HaveOccurred())

	mgrStopCh = StartTestManager(mgr)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	shutdown = true
	close(mgrStopCh) //tell the manager to shutdown
	waitGroup.Wait() //wait for all reconcile loops to be finished
	//only log the error until https://github.com/kubernetes-sigs/controller-runtime/issues/1571 is resolved
	err := testEnv.Stop() //stop the test control plane (etcd, kube-apiserver)
	if err != nil {
		logr.Warn(fmt.Sprintf("Failed to stop testenv properly: %#v", err))
	}
})

var _ = AfterEach(func() {
	testFinished = true      //to not trigger other reconcile events from the queue
	Eventually(func() bool { // to wait until the last reconcile loop finishes
		return reconcileInProgress
	}, longTimeout, mediumRetry).Should(BeFalse(), "Test didn't stop triggering reconcile events. See operator logs for more details.")
	CleanUpCreatedResources(cassandraObjectMeta.Name, cassandraObjectMeta.Namespace)
	mockProberClient = &proberMock{}
	mockNodectlClient = &nodectlMock{}
	mockNodetoolClient = &nodetoolMock{}
	mockCQLClient = &cqlMock{}
	mockReaperClient = &reaperMock{}
	testFinished = false
})

func SetupTestReconcile(inner reconcile.Reconciler) reconcile.Reconciler {
	fn := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		// do not start reconcile events if we're shutting down. Otherwise those events will fail because of shut down apiserver
		// also do not start reconcile events if the test is finished. Otherwise reconcile events can create resources after cleanup logic
		if shutdown || testFinished {
			return reconcile.Result{}, nil
		}
		reconcileInProgress = true
		waitGroup.Add(1) //makes sure the in flight reconcile events are handled gracefully
		result, err := inner.Reconcile(ctx, req)
		reconcileInProgress = false
		waitGroup.Done()
		return result, err
	})
	return fn
}

func StartTestManager(mgr manager.Manager) chan struct{} {
	stop := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(BeNil())
	}()
	return stop
}

func createOperatorConfigMaps() {
	cassConfigCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorCassandraConfigCM(),
			Namespace: operatorConfig.Namespace,
		},
		Data: map[string]string{
			"cassandra.yaml": `authenticator: PasswordAuthenticator
authorizer: CassandraAuthorizer
auto_bootstrap: true
auto_snapshot: true
back_pressure_enabled: false
back_pressure_strategy:
  - class_name: org.apache.cassandra.net.RateBasedBackPressure
    parameters:
      - factor: 5
        flow: FAST
        high_ratio: 0.9
batch_size_fail_threshold_in_kb: 50
batch_size_warn_threshold_in_kb: 5
batchlog_replay_throttle_in_kb: 1024
concurrent_counter_writes: 32
concurrent_materialized_view_writes: 32
concurrent_reads: 32
concurrent_writes: 32
counter_cache_save_period: 7200
`,
		},
	}

	shiroConfigCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorShiroCM(),
			Namespace: operatorConfig.Namespace,
		},
	}

	prometheusCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorPrometheusCM(),
			Namespace: operatorConfig.Namespace,
		},
	}
	collectdCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.OperatorCollectdCM(),
			Namespace: operatorConfig.Namespace,
		},
	}
	Expect(k8sClient.Create(ctx, cassConfigCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, shiroConfigCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, prometheusCM)).To(Succeed())
	Expect(k8sClient.Create(ctx, collectdCM)).To(Succeed())
}

// As the test control plane doesn't support garbage collection, this function is used to clean up resources
// Designed to not fail if the resource is not found
func CleanUpCreatedResources(ccName, ccNamespace string) {
	cc := &v1alpha1.CassandraCluster{}

	err := k8sClient.Get(ctx, types.NamespacedName{Name: ccName, Namespace: ccNamespace}, cc)
	if err != nil && kerrors.IsNotFound(err) {
		return
	}
	Expect(err).ToNot(HaveOccurred())

	// delete cassandracluster separately as there's no guarantee that it'll come first in the for loop
	Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: ccName}, &v1alpha1.CassandraCluster{})).To(Succeed())
	expectResourceIsDeleted(types.NamespacedName{Name: ccName, Namespace: ccNamespace}, &v1alpha1.CassandraCluster{})

	cc.Name = ccName
	cc.Namespace = ccNamespace
	type resourceToDelete struct {
		name    string
		objType client.Object
	}

	resourcesToDelete := []resourceToDelete{
		{name: names.ProberService(cc.Name), objType: &v1.Service{}},
		{name: names.ProberDeployment(cc.Name), objType: &apps.Deployment{}},
		{name: names.ProberServiceAccount(cc.Name), objType: &v1.ServiceAccount{}},
		{name: names.ProberRole(cc.Name), objType: &rbac.Role{}},
		{name: names.ProberRoleBinding(cc.Name), objType: &rbac.RoleBinding{}},
		{name: names.ProberIngress(cc.Name), objType: &nwv1.Ingress{}},
		{name: names.ReaperService(cc.Name), objType: &v1.Service{}},
		{name: names.ShiroConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.PrometheusConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.MaintenanceConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.ActiveAdminSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.AdminAuthConfigSecret(cc.Name), objType: &v1.Secret{}},
		{name: names.PodsConfigConfigmap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.CollectdConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: names.PodIPsConfigMap(cc.Name), objType: &v1.ConfigMap{}},
		{name: cc.Spec.AdminRoleSecretName, objType: &v1.Secret{}},
	}

	if cc.Spec.RolesSecretName != "" {
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: cc.Spec.RolesSecretName, objType: &v1.Secret{}})
	}

	// add DC specific resources
	for _, dc := range cc.Spec.DCs {
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DC(cc.Name, dc.Name), objType: &apps.StatefulSet{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DCService(cc.Name, dc.Name), objType: &v1.Service{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ReaperDeployment(cc.Name, dc.Name), objType: &apps.Deployment{}})
		resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.ConfigMap(cc.Name), objType: &v1.ConfigMap{}})
	}

	// add Cassandra Pods
	for _, dc := range cc.Spec.DCs {
		for i := 0; i < int(*dc.Replicas); i++ {
			resourcesToDelete = append(resourcesToDelete, resourceToDelete{name: names.DC(cc.Name, dc.Name) + "-" + strconv.Itoa(i), objType: &v1.Pod{}})
		}
	}

	for _, resource := range resourcesToDelete {
		logr.Debug(fmt.Sprintf("deleting resource %T:%s", resource.objType, resource.name))
		Expect(deleteResource(types.NamespacedName{Namespace: ccNamespace, Name: resource.name}, resource.objType)).To(Succeed())
	}

	for _, resource := range resourcesToDelete {
		expectResourceIsDeleted(types.NamespacedName{Name: resource.name, Namespace: ccNamespace}, resource.objType)
	}

	nodesList := &v1.NodeList{}
	Expect(k8sClient.List(ctx, nodesList)).To(Succeed())
	if len(nodesList.Items) != 0 {
		for _, node := range nodesList.Items {
			Expect(k8sClient.Delete(ctx, &node)).To(Succeed())
		}
	}
}

func getContainerByName(pod v1.PodSpec, containerName string) (v1.Container, bool) {
	for _, container := range pod.Containers {
		if container.Name == containerName {
			return container, true
		}
	}

	return v1.Container{}, false
}

func getInitContainerByName(pod v1.PodSpec, containerName string) (v1.Container, bool) {
	for _, container := range pod.InitContainers {
		if container.Name == containerName {
			return container, true
		}
	}

	return v1.Container{}, false
}

func expectResourceIsDeleted(name types.NamespacedName, obj client.Object) {
	Eventually(func() metav1.StatusReason {
		err := k8sClient.Get(context.Background(), name, obj)
		if err != nil {
			if statusErr, ok := err.(*kerrors.StatusError); ok {
				return statusErr.ErrStatus.Reason
			}
			return metav1.StatusReason(err.Error())
		}

		return "Found"
	}, longTimeout, mediumRetry).Should(Equal(metav1.StatusReasonNotFound), fmt.Sprintf("%T %s should be deleted", obj, name))
}

func deleteResource(name types.NamespacedName, obj client.Object) error {
	err := k8sClient.Get(context.Background(), name, obj)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return k8sClient.Delete(context.Background(), obj, client.GracePeriodSeconds(0))
}

func getVolumeMountByName(volumeMounts []v1.VolumeMount, volumeMountName string) (v1.VolumeMount, bool) {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == volumeMountName {
			return volumeMount, true
		}
	}

	return v1.VolumeMount{}, false
}

func getVolumeByName(volumes []v1.Volume, volumeName string) (v1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == volumeName {
			return volume, true
		}
	}

	return v1.Volume{}, false
}

func getServicePortByName(ports []v1.ServicePort, portName string) (v1.ServicePort, bool) {
	for _, port := range ports {
		if port.Name == portName {
			return port, true
		}
	}
	return v1.ServicePort{}, false
}

func markDeploymentAsReady(namespacedName types.NamespacedName) *apps.Deployment {
	deployment := &apps.Deployment{}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedName, deployment)
	}, mediumTimeout, mediumRetry).Should(Succeed())

	deployment.Status.Replicas = *deployment.Spec.Replicas
	deployment.Status.ReadyReplicas = *deployment.Spec.Replicas

	err := k8sClient.Status().Update(ctx, deployment)
	Expect(err).ToNot(HaveOccurred())

	return deployment
}

func validateNumberOfDeployments(namespace string, labels map[string]string, number int) {
	Eventually(func() bool {
		reaperDeployments := &apps.DeploymentList{}
		err := k8sClient.List(ctx, reaperDeployments, client.InNamespace(namespace), client.MatchingLabels(labels))
		Expect(err).NotTo(HaveOccurred())
		return len(reaperDeployments.Items) == number
	}, longTimeout, mediumRetry).Should(BeTrue())
}

func createCassandraPods(cc *v1alpha1.CassandraCluster) {
	nodeIPs := []string{
		"10.3.23.41",
		"10.3.23.42",
		"10.3.23.43",
		"10.3.23.44",
		"10.3.23.45",
		"10.3.23.46",
		"10.3.23.47",
	}
	createNodes(nodeIPs)
	for dcID, dc := range cc.Spec.DCs {
		sts := &apps.StatefulSet{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
		Expect(err).ShouldNot(HaveOccurred())
		for replicaID := 0; replicaID < int(*sts.Spec.Replicas); replicaID++ {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sts.Name + "-" + strconv.Itoa(replicaID),
					Namespace: sts.Namespace,
					Labels:    sts.Labels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "cassandra",
							Image: "cassandra:latest",
						},
					},
					NodeName: nodeIPs[replicaID],
				},
			}
			err = k8sClient.Create(ctx, pod)
			Expect(err).ShouldNot(HaveOccurred())
			logr.Debug(fmt.Sprintf("created pod %s", pod.Name))
			Eventually(func() error {
				actualPod := &v1.Pod{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, actualPod)).To(Succeed())
				actualPod.Status.PodIP = fmt.Sprintf("10.0.%d.%d", dcID, replicaID)
				actualPod.Status.ContainerStatuses = []v1.ContainerStatus{
					{
						Name:  "cassandra",
						Ready: true,
					},
				}
				return k8sClient.Status().Update(ctx, actualPod)
			}, mediumTimeout, mediumRetry).Should(Succeed())

			Expect(err).ShouldNot(HaveOccurred())
		}
	}
}

func createNodes(nodeIPs []string) {
	for _, nodeIP := range nodeIPs {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeIP,
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: nodeIP,
					},
					{
						Type:    v1.NodeInternalIP,
						Address: nodeIP,
					},
				},
			},
		}

		existingNode := &v1.Node{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeIP}, existingNode)
		if kerrors.IsNotFound(err) {
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
		}
	}
}

func markAllDCsReady(cc *v1alpha1.CassandraCluster) {
	dcs := &apps.StatefulSetList{}
	err := k8sClient.List(ctx, dcs, client.InNamespace(cc.Namespace), client.MatchingLabels{v1alpha1.CassandraClusterInstance: cc.Name})
	Expect(err).ShouldNot(HaveOccurred())
	for _, dc := range dcs.Items {
		dc := dc
		dc.Status.Replicas = *dc.Spec.Replicas
		dc.Status.ReadyReplicas = *dc.Spec.Replicas
		Expect(k8sClient.Status().Update(ctx, &dc)).To(Succeed())
	}
}

func waitForDCsToBeCreated(cc *v1alpha1.CassandraCluster) {
	Eventually(func() bool {
		dcs := &apps.StatefulSetList{}
		err := k8sClient.List(ctx, dcs, client.InNamespace(cc.Namespace), client.MatchingLabels{v1alpha1.CassandraClusterInstance: cc.Name})
		Expect(err).ShouldNot(HaveOccurred())

		return len(dcs.Items) == len(cc.Spec.DCs)
	}, longTimeout, mediumRetry).Should(BeTrue())
}

func waitForResourceToBeCreated(name types.NamespacedName, obj client.Object) {
	Eventually(func() error {
		return k8sClient.Get(ctx, name, obj)
	}, mediumTimeout, mediumRetry).Should(Succeed())
}

func createReadyCluster(cc *v1alpha1.CassandraCluster) {
	createAdminSecret(cc)
	Expect(k8sClient.Create(ctx, cc)).To(Succeed())
	markMocksAsReady(cc)
	waitForDCsToBeCreated(cc)
	markAllDCsReady(cc)
	createCassandraPods(cc)
	for _, dc := range cc.Spec.DCs {
		reaperDeploymentName := types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace}
		waitForResourceToBeCreated(reaperDeploymentName, &apps.Deployment{})
		markDeploymentAsReady(reaperDeploymentName)
	}
}

func createAdminSecret(cc *v1alpha1.CassandraCluster) {
	adminSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.Spec.AdminRoleSecretName,
			Namespace: cc.Namespace,
		},
		Data: map[string][]byte{
			v1alpha1.CassandraOperatorAdminRole:     []byte("admin-role"),
			v1alpha1.CassandraOperatorAdminPassword: []byte("admin-password"),
		},
	}
	Expect(k8sClient.Create(context.Background(), adminSecret)).To(Succeed())
}
