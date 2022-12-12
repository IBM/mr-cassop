package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	caCrtBytes, _      = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZGakNDQXY2Z0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFkTVJzd0dRWURWUVFLRXhKRFlYTnoKWVc1a2NtRWdUM0JsY21GMGIzSXdIaGNOTWpJd01qRTNNVE14TURJM1doY05Nekl3TWpFMU1UTXhNREkzV2pBZApNUnN3R1FZRFZRUUtFeEpEWVhOellXNWtjbUVnVDNCbGNtRjBiM0l3Z2dJaU1BMEdDU3FHU0liM0RRRUJBUVVBCkE0SUNEd0F3Z2dJS0FvSUNBUURJYnNFQjFNOVFIa0h6bld1S0VFUE40NHV4WEg4cG5vRDFsYWQyeFRWNUd3eHIKWnhtQmhIY1Y1Tk1ZZFJUbWEvYksyclZHbi9vQkhqRGY3VWVvOUlqUjVlTXFYc2tqYTZZSTJ3UGxuK3ZtR29hagpsQ0xOUmEwb01Xa1JFM0RJSStTbUthNk9LTDB6ek9oK3dnN2dBYVhqa0VxbXRVVStqcHBJY1VPbXgvcmM3Qlk2Cmxvc3dmWXVmN1MvTE9CY2N1WFBZd214SWt2MEkrSFhDMjdmcVQ0ZmlzV2xCYUtxV2VINVdBcWpsd2FHMVUvMW4KR1NmTHYrcW5OZERkWkwxeERlcDljWW40UitLcTFvQjE0b3VZaklXUVdSUk9UNHFuWHAxQ25kanNXZDVoUitWeApIL1Vac2RiOE9zTm9KOUVEYXNVMmtsYnljVGJjNE1UbSt0RUdOYzdNNVNMT0dHY3BpMzhzV2dEeTNUKzI0N1NUCkkycDl6WWdOdWYxcHZmNW8zbU1KWW0yVndiUC9qUHk2bmlRaXFJQ1FyUzU5d3hsaTBiTVFHN2E4ZWIxT2dPd0UKdHlGT1RVRm9kemdLVDQzbzVJanF4blE0TEZWZlZXYytHakgza3JQOEk0RGJSRW56bERUMDhXWE4xYVFMZmwvSgpBb2tzbFZzc2l1d1lza1h5Z0M1SjVONkdrUkhWa1pUZVNZY3Rna0x4MDFCeHNJSFJIV2E2M29CMnZFc2Nnanc0CnQ4TDZncHFBUGZlTlhIVFlORzJYeTdsZGlYNUJsR3QvT2FpU1hRZE1ZRHpuT3ZrOXAwRWt6VGJpdkRveHJkZlgKeWxnbHJESjBXcWJPcDUrdVhSVGFXaGt2TUc1cnJLMDk2NXU1c3BaanU5ODNZWk56eW9aTk1YK2FEYkswc3dJRApBUUFCbzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSEF3RUdDQ3NHCkFRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRktzb2RIQW9lczFDVHVDUmRYTDEKa0FPdkZKdUVNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUNBUUF3a2RVdmZjTytOMWxIcFVXOGh3WE5FZkZRVDl5MQp2dnd4NGc1ODNNckRhNDVINmwvcU5waTVzbjBHbkF6aHNHL2swNHdleURwTEJUR1Fha3BQczVsRHA1b1NpL0l2CkVwWXE5bmhJcXZXbTlzYVpQUXQrVUpxTjcxK1VUUFdFOGZlQXFJSVZJQlMrdTVrNVZRQnVud2JIakJLU050cVAKVC9iYmdldjdxZG5ycEd2YWtpdlFxU2hmQUlyRGR2TGNJSlNNQnhIK0VvT2M1Y2xsUVpRRlR0Q29OYTR0THpPTwpVR2czWGVGRmYyMHIyTXE0U2pYNURzMHBIcTYwSC9DMGJZYmdxQjBFcHBNSXRwdTBZbndGUWlzdGY2OE5LV2xqClAxVG80a0d0bVBkdmo5NU9uVk5PUlFSYlF2NktwcU55R0hSdEM0MWpUbVV5WmVVK1k0Z1c4b2lXVHB4eDZEZHQKN2lTOU9FT0g2SXVocE1WYXdEdjFYdThYQll4NHFVRjRwNnVqVWRuRUZTVW9VQi9vWXVXVkQ3UFAzU3E5SHVVNQpXQTgybzF6UmY1OXF0Z2NsV3ZNTEJQWWtpR29CekhNVFQ0NENBcTRXRkZ0bTA5Y0JPdC81VHRYZ2xhSkhvRmRDCmpnc0tlWjhUYm15Q1VnWHZ4VDlpWkNnYTUzTnhxMGFudGRDUkNQbisrTE5lcFZBRkFuYW54eWdBT0V6cW5SdXgKQm1XNDdvbTNCdGdrMnFiS3FjMnI4anZtN3A3ajRoZFJPTW5GRHV6c1RPdVNnRlFBRWtWenMxYjEwdWFGbldWNApISURySXN2VzlQZjBBR0J3S1B1dVArUVNoME9QU25JUEZrNWM3U1gwSmlBelp6VHJGWXUxYUhaTzlXMHRPTFk5Ck1uaTBtYWVucDRyMXB3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=")
	tlsCrtBytes, _     = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZQekNDQXllZ0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFkTVJzd0dRWURWUVFLRXhKRFlYTnoKWVc1a2NtRWdUM0JsY21GMGIzSXdIaGNOTWpJd01qRTNNVE15TlRFeldoY05Nekl3TWpFMU1UTXhNREkzV2pBeApNUnN3R1FZRFZRUUtFeEpEWVhOellXNWtjbUVnVDNCbGNtRjBiM0l4RWpBUUJnTlZCQU1UQ1d4dlkyRnNhRzl6CmREQ0NBaUl3RFFZSktvWklodmNOQVFFQkJRQURnZ0lQQURDQ0Fnb0NnZ0lCQU1nd2JuUkIzVFNUT2Q1V0JORUMKSVludlZGSGtuRDkwZ1NpZjFOK0VaN3hlWGkyZEd2cWxOazN2RVhIS1pvZ2RQWGd6RHR5aXdHaHJLL1BJdWhYdQpsSXdZWnNFbFNObzNXWnBiSExOLzRvZVVVNXJLbHhkV3BvaTN3MlVCdGIrVkx1L2FTbnpPdk82UHV5ZGxJcnowCjM4L2FQbWRteGhLRWhhbzJCdnBQNkE1MUc4bmY3OHhRaGVHQUJDcjdZQXUycE1kSHdSdEZoMEtGRDJIaFhtbTYKb2lhT3lxTkN0WUUzNU11OWNuRFp6Y3I1ZnBHakQ1TXJnSm00QVB5aWc4Mi9uczlOWlRsRUEvOVBac2x5NjlUcgp2ZC9iaXRzeVBGeVZRRTBoZ2JjTmU3OHNLMStIZEI4YkJGaW8wZnoxVVBhUnVib3RHcjRab3VCK2QySHUyQndRCjFBTnBQQytEbWY1OFZrekZ5SDEwSk9rNnhjS1BiaExpK3dOWk9DYnFhcnVQTWh4c0grNTJkNWRwNnk5d2FqS0QKTWVnNjFRWlU0eGVmc1JQa0FoR0p2RS9VRXlIY0tzb1g5NlozNVRyb2ZOSy9saEUrKzljamxZM0UzNkhFRy92egpURE5UNlNOYTNocFA4QTFsUW0vNm4xcVdtNXhvSWM5cGJTdHZTUjN5UkRURnlmZTM0TVJIZnl3YUIxOU1FZFNBCnZPSGIxalVqelRVVmsyLzd2RVRnUmwyYjFFSVFHeVBBdzlsUXF0ZnAvMmlyMHJWaE5sSUdWcWNSQkUrd0lLZnUKdHlYcE9WQXRuUkFVb1BKR2paRVQ0UnJRS0RLeTJKWnFJY0RQL2J5MDBqMkJJTG9NczFLbitsaW5yRlovd1VtVwpUM21ocUlWT2UrTzROOTBaeC9XcVcycG5BZ01CQUFHamRqQjBNQTRHQTFVZER3RUIvd1FFQXdJSGdEQWRCZ05WCkhTVUVGakFVQmdnckJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0RBWURWUjBUQVFIL0JBSXdBREFmQmdOVkhTTUUKR0RBV2dCU3JLSFJ3S0hyTlFrN2drWFZ5OVpBRHJ4U2JoREFVQmdOVkhSRUVEVEFMZ2dsc2IyTmhiR2h2YzNRdwpEUVlKS29aSWh2Y05BUUVMQlFBRGdnSUJBQkI2RGZhL2NGNTRWb25TbFRJdm0yV3lyc3k4WXllRG5SemZtOFNHCktqNWVKMmxEb3JpaVJVUDZuRE5LM3dRWjdzRXJMZSt0L3NpUmVIWFpkbVB0dHFpSVlxWStub1Fjd1AwYTJXMXkKUlJIbUY5SVM5eGZDQzlHeEtuNlBSNWNYYTBVM2lvbStRRUxVWVYxbXlaRGdMdXZENjBLVlphTDFJWlJBK2Z6bQoxWDh4Q0RTN0lQNXRibFBuZVJKb0t2UlJpRm1ERzdtbXB0SFUxcEs3Zm1yWlZCZzFKSXdpNWVVRVJHNzRoU205CkFhVkpyeEY3SnUxMFplN2dNSG5EeUJLSUt1U1g0NXFhaUJWQlRsSGVJSk1SQStQTk5NT01JaWRvanhHRi9pVSsKZjljRzlhSnNDRHBNalJYcG1mU1FSYk5wdzlPM2pDR0NyN0xwS2EwL2xJUDBIWmp1bDcwTzFwK0Z5UTZGaXZKdwpOTWdiSHdWV25PZXU0UUxVb1RQRGIyVGQxNU5qaS9CaG9vVy9qdGRHZDF3dmxjWlcxVW91ekI1T2VOOThyT0NSCnQ3UkZHR3U2NnFtS250ODZWajhra0dwOVdqeW55Z0p0Z0tFTDVYZWF5OXRHS0dJTEhzQ1FvRmdSVUc5WkZHeUcKWEsvRU1MeWtEb1QrZnhDUWZoTU1SSmVOMVdpc3BjdnBqMnl5RytIOWxraXJwTm42WFhvS0tSV200ZjVDS0pOSQp1TTNSU2lvS1pReDk2WVUvZFpRUDVtVk9pMjkyRm0raXFvMkxPQmt6dlNDVVVKZWdjQXU2dHdmL2ZOUnpFaThGCnRoaXZiNU55cUprcGoyS2djWDc4Y3drVm5ndHRzMEdoUVNhdDc0bHZqQTltc0k5MkZuSmZLQXcrK1F0ZHZiRHkKR3JCQQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==")
	tlsKeyBytes, _     = base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlKS1FJQkFBS0NBZ0VBeURCdWRFSGROSk01M2xZRTBRSWhpZTlVVWVTY1AzU0JLSi9VMzRSbnZGNWVMWjBhCitxVTJUZThSY2NwbWlCMDllRE1PM0tMQWFHc3I4OGk2RmU2VWpCaG13U1ZJMmpkWm1sc2NzMy9paDVSVG1zcVgKRjFhbWlMZkRaUUcxdjVVdTc5cEtmTTY4N28rN0oyVWl2UFRmejlvK1oyYkdFb1NGcWpZRytrL29EblVieWQvdgp6RkNGNFlBRUt2dGdDN2FreDBmQkcwV0hRb1VQWWVGZWFicWlKbzdLbzBLMWdUZmt5NzF5Y05uTnl2bCtrYU1QCmt5dUFtYmdBL0tLRHpiK2V6MDFsT1VRRC8wOW15WExyMU91OTM5dUsyekk4WEpWQVRTR0J0dzE3dnl3clg0ZDAKSHhzRVdLalIvUFZROXBHNXVpMGF2aG1pNEg1M1llN1lIQkRVQTJrOEw0T1ovbnhXVE1YSWZYUWs2VHJGd285dQpFdUw3QTFrNEp1cHF1NDh5SEd3ZjduWjNsMm5yTDNCcU1vTXg2RHJWQmxUakY1K3hFK1FDRVltOFQ5UVRJZHdxCnloZjNwbmZsT3VoODByK1dFVDc3MXlPVmpjVGZvY1FiKy9OTU0xUHBJMXJlR2svd0RXVkNiL3FmV3BhYm5HZ2gKejJsdEsyOUpIZkpFTk1YSjk3Zmd4RWQvTEJvSFgwd1IxSUM4NGR2V05TUE5OUldUYi91OFJPQkdYWnZVUWhBYgpJOEREMlZDcTErbi9hS3ZTdFdFMlVnWldweEVFVDdBZ3ArNjNKZWs1VUMyZEVCU2c4a2FOa1JQaEd0QW9NckxZCmxtb2h3TS85dkxUU1BZRWd1Z3l6VXFmNldLZXNWbi9CU1paUGVhR29oVTU3NDdnMzNSbkg5YXBiYW1jQ0F3RUEKQVFLQ0FnQlhML2k3TzRCYkNKQVlqSUEyZzJZV0RCMCtPWUh2aFE5SG9TejJXRlZSemd2WWMrY3ZLTXRZNy9rNgpCUHZZNWx0Q1FZS0VoNEdqT0tJQTMvaGoydS9wZ0Nzb2pkT0g3cmxncDdQOVhZSk1xRVl0VVhTeW5uT1RjZTF3CnpXalowdUNKYUJvdTkxK0R5eWVveGJ4MmJYUVlzNldnSlR1NUR1eWtNRG1qeFM5OU5IWHcyMDQyUHcvQUlhQXYKRkNKRmw3dDJhNExvSE1VSExLcUJaOFZWNlFuOEh3NlVRbGFJdTg0ekNnU1FyOXVZUGtkb3FJcjgvdUpZa0xJbApWYVp2OHJmNDgyMVZucERiSGpyWEcrMG9ZNi9qa2x1TWtmbUdIUjdQT1B1a3ZRT2JSR0p6amF2K2JDYmRjbEZhCmlZRkVrajFBbjZwWkJ4TlFFT1J2eTdWbHE5S2FEMXdzUEExck1MS2d0WHMraW5kNjlSb0g0Z0lHMWVyNDVyWEgKeFgyUzhlUEMxVnZPczVyMTFnbjlDTHZGM1g5TEkwRzhyUTdmMnhHT0JmUGZSRFNtQndia1NGRFQ4NWtKcmU4YgpxU0dWS3pnVmZLQitkWCtGc1pyMnowVzg0WkorNzFINXJNVmwzRFhUZDVvWm44MHMwTWJ2Vm0xQTR5ZThoS1haClVyWUlKaGk5aFBQTGRFdjkva215M0lZQ1RPeVA3M2lxQWtxWFROQk1sMU5nMXJEVnFnOEJMbWlQQnhsYkdEWkwKMUJiVEp6SGxEdUh0VER0aEgrTlFFamJtL2J2YlVWaWg5SnIyYklKd1RtZHJDOURubGlZckhPMVNERk9NelpXMQplZ3RoZnU2N3QxaTFEZzB5RXZ3QTRjd0l6N1RnSStZMWhWOFdnU1pMekxzazdVbTVJUUtDQVFFQXl3MW15TFdaCkExNHZKbDNDV2JGUGd4T3hwV1daTUlsTUUySDhBNElBY2xMcmZYYUVDaEZFYUZLUEhSNVc3M01GWkk1ZGI4ZCsKVDNnNHRWSFgvRm1kRHFWbkZHVEVjdlF6TjFFNDV2S2M5akNDYVNjYmppckxJMi93dGJmbjUxK20rZ2FZSHUyQgoxMWVQS0NjVlFMU1YrallSSWFvbzhwUXhMNTJBZzB3TFMwOFJWcDltSjJYZVJNRmw0Z04yMWRHODFBSHo0cE50Cks0TExjREh1WFVxaWhVb1pvSDNKdXVnQWw1OWpGaDI4U0d2N1Y5dzJybldLUjdEaWwrajZqUXJlTUJsVys4V1IKeXZ0TW5ZQUk2YXJkRkg2TEtRbzkyaDFTbjlxUFczQnU0WGZoS1cyTS8vS0Jmc0VmZXJSYWNPY0dNM2ZYQ21uUwpNd1BjaXZoRDdzdHFOd0tDQVFFQS9HUG0xakphSW9CZU1xL2oyZ3VuNGh3V2ZoVjluZkZtTGJzaGhKL2hvNTFZCkFrRE5PSzNQdzVQaGpNVmF0OXBscE5LT3dKdTZ6Y2lKbXY5L3lZZ3JIRFFsZHI5TlN5TFA0akRzUEgyTllRWG8KTWZ5aWZSbzZBWDhkVE05S25GampaN2RBUFA5Y20yWGR6c1pvWG0xSFBCbDhqZFByY3hXQXRSYVdiSGtMOWVjRwpGQzY4VHFjTDJ1MHdocjFwK1Q3Ukp5U3FmTkdlbDBJdTkwSG5SMGUyWE1UeWRSSHhtY3c0a1pHcXZ0RVBqcmlECkZmK1FYQUlJdjFrNGJ1M2xwWmlUTWQrOE94U2JkK1JlVXF6MW5wb2YyMm8zQTJsck91V1lGdksxWWc1b3dzTUsKSlBkVGgyclk2RHYyblp6WUZKTitnQWEyRzhiV2xWbFhHNVErQ2VRcFVRS0NBUUF6QmNjU0lDKzIzTy9VSURNNQovb2dRVTk2THhSL2RSbUxMYkErN2dldEN6dnRGcDRZK1VXQnpKbXUzMFd2ZTkzUWZkSGFlSSs3bFlUdytzN1ltClB3NXNJN3ZuTC9yOU44ZTIycjRGcW1rdW83bmhXbVplTHZxa2lQaGZjVHJndFBSc1YwUGFZYXdDeEluQWVUR3AKOUxiTEROTDVOcHpiZDhsMFFZdU5jb3BUL2laSk1meWxZYitjWDB5T29uZzErbUlNbEFFSXFpc0FoL2srMUEwbApmQitpaHFBeDUrbk5BWWRFa2xSL3RqRWRzYVNTeTA4aVAzN2p6TnJqZVRPY1JXTFhpMGFFTlgwUkdGeUVKeXdXCkswMHFYVEh5dWhRWXZzMklSWTlJVnRSRjY2MjBqMGFpK2ZqNE5PY0lHbEd2bFIzRnlSTk1GcE12Sm9WQWxtbmIKazFtNUFvSUJBUURvL3Z6NkhvK3hMQnBKNXJDTStaZXBtRTUzWlRXTEduQStwWE5pOFFvbnRqaXFFOUtna1d5cwoyNWNzRW9FV0cwc2NacmtjUEdldVU0UWREK09UVTk2Z2NjOW9HV0NzalYybUxZM1lwZnRmWjBtYzQrSEpaZTNJCnRlV0JwQmYzeitOWE93ZlZoOVNoTE9BZ1pHK3pSR1N2cWNPZlZ0VVViL1JhdUpoeldTZmVnY0ROM3ZzK0RONVgKNUFHWlVjRkVOR1ozSVZVMlYzbDFuOGFsd2pSVlRIR2dvTHhIc1NIOTNsY1dvNGdpRWZPdnlNeFRBWlB1TEg2UQp6emZXeUUzcG1ybkFJQkN2NWUxRU9CR1BkMVprYlZvZGY2ZDd3ZFVaRVIxZmlmNUNvSlM3djd4Y3RlcHBxQXpJCkQ1c3lrN215L1JxWjdCZ3YrbGJjbHhaZ1FuRW5SbmVSQW9JQkFRQ2lIUHg1NFA0aGd4S1hjYTBXVTh4cHhKeEIKQXF6eG5KTUVNdUNSOFdQZjVaYlc0RHFpanBHU0M4UzhML2dXS2VYaVpLZFhBUUx1ZW9wQmhIcGZpZnZXZjRFVwp5WnhZbm00V015U2VpS093V1YwT3Z6MDRNUkJxek5FdFIvMGNBOGFJa3MxNkF1MWlwNDhySWJKZkxsMGpIMXJTCm1hUmdBWEQ1bzQrblp5Rk5MZS84RmtYMEJjQTBIMUF0d1BOZ3k1YVhNSlh2SWs2U0IzaFpRS1h6Z3lObUUzSnkKWlVMTHNGMGpFWVFDUU9ZZk55bGF1MnpGelhvMU1oQk9leGJ6YTVGVVhGcDFkSEZNQW9ESnh6SGgzRkw0K0JZdgpKUklxb3g4S0hXYlZqMzNSc1NpbTVWVlJPVUFmai9iZXJKeGErV2dqbnorYzVOL1NHRlRIVEl1MkJiMEQKLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K")
	keystoreBytes, _   = base64.StdEncoding.DecodeString("MIIVQwIBAzCCFQ8GCSqGSIb3DQEHAaCCFQAEghT8MIIU+DCCCy8GCSqGSIb3DQEHBqCCCyAwggscAgEAMIILFQYJKoZIhvcNAQcBMBwGCiqGSIb3DQEMAQYwDgQI4UNNcSnNN3ACAggAgIIK6BYW0hFF7R8jQUSwwFQgYtLguH8e6oCcrYBSVFGDlWja8pEkL5g2z/UoUD1tRV0KRzle3njAPIIl5HMdQNH+QCe9CUMzLMrtk6YCkP6pQYUQdq3MYCAx+UdI1nxuEmeNtSng/p+i6AlnqRu1HIwzxKoiep//uqHRKbxiU9+F+KUWVoI0+AH00ofWk0+JWeiGcye7jCIHTeUG3ERYJMkkJ2d1WJm5rSXulvgPdFuyH95srZcgpCxXa9aqrM40RQ6UYqUesvCI4lktzUOkIayuauBC6h9LzRrgZSXMzOSrae3VhqyHZtSdUNhgQfN3qGuSyztfbN2XWWnhO/HwTPeWz5MlfrPbhV+IdPSgrcWC57EMIT/3l4xDnXajW/VdSBnN87GjRucpnKxs0i4dHkFWq0ExhCYFjieWuKsIKWEqGwXa23LNGG1ZUbkYJ2vkkhxukA6vozhhiKr1+C8Vtz210Xbr/Mgga/ZIsCkhHEEFZ7YmISB8yZjjX4WiSrUGIRhr+SIiH5sJXlYSPTxZ5eMHFhD/lJpGrh/cl3F8nqVjAwOcfrWc7D7KW9hwIlLiKF+fMgYWoLe/5u7mVZ9B9A2CoeNBijauYeFjwQ7IDWTC1h7JONLUejdNAOfxU8ckUn25n8m8H9ku/oU0pK8ZS0iCx5az6mX4MF8KacXYZTd08xfKiZPguDeA36zKl0qrfqHxKqOlFWvZbLXLdQHI5ZGfDK5x6sYvOlxrHBM7cRqelx2ZMWiH58Ss8pkaebKd42QyHKI2XlVFkNO36fyfVsdIfPljv9eiHYOrBMiFcm+1Ow0FKYDk8ir8Qdg6yKsEmGY53w3c+hQf0e6+fTW7zHwA7FT1G/QYTNkyGOzrZ20YfQ+8CGuDtqdf9BD5aoHy4HMvKJtTE9XohHpzS8aLc8sL/t6VmzyVOup7aTohc0ggEJnXq6DL/+2LWpd39MNTTF65xgMtYqkKxUyEG6iE0H8HAjgeY/FWIIerikdHv34jJQS1DsrZhNn8eDIsGSCDlJODdqwzFRwkW6kpB5/aBZqpJg46ViGHaDxfPmc35fL7o4HQfxz+UaG0b5KgvyhntApJ9NkvjML8DG/xk38ENhPhjRvYczVFeOX+U3XKJ9eJhQvcHfw6T4XsFvcMHXJoKKr0HIILvKjuqZ6LmAy33GeC9wLULfrmMRuFmlfMcJLUSVR+PK5QiX9HG8J/GFiz5hdU5dxxRaQwqEuQrpIyA892EVavNcAnS6yUerMUhyxiwL69PPx10ZZBJ5NtBgfP5BZrLqxVRzbEPaMlHuvwoznFGFGEZe9qOf6ReOLWb82dagsP+Q6KWvVJw2D3j7iys5iFdbuVmCt4/V4uBxsDnzwXx5dGJEwTqSqGtzYO93/ysA8RhRl923rwC6mm7TQ2cCy9Awsb8Stx9v4yhSGb6RnPtzk7m8BB/+D0id851rScwoAoKWXGVngDACo5WS8kKD4dOMY/kc5y3a0wSQmgqhV/XSlPORaPiNS11JtI8cKbp5cFukIlTyBGYP56JXW41OGqpi36G7+3WiePI7XEiljlLfNlCEsD8O24k892hNAmOC5GZ38J57898HrmYHBAUw6LA2xs+SSxptJGYHea4B/N9sJXU8aDrLWe1k9g6ERu1N41KeVQm7VIP0b7UHig+mR21d2LCvD/aE3KGzCd+vJkAIQKhk2ItS0LETvf7r06tjlR6TVoT0jL6PCmJkEoV1bPF7SA64LAA6LE3/lTxH33GgG57AHQ+Tc2e2J06fslx8cIZZ3DYg6LUza0n9fzrTVFGBzDX/dcC+5VXxRmQLJBaHhGVXcWBusY5fP9n+FQtjm2rL9HdyjioUJizBs2gozVgm/oKzsCzPM7w0lcVvpQ64et+OQ/IhryFZ+BkdEL7sggAbzxvuLR/ovC5pDursC3xWpMDReZTqKKOXmly/pFpbymOxC2TtK9JIYTyYDk0IsHpIPk5PSzz8bR0sc33rS721JsnIREzTtsavHyuizIuya91J3Seq3wAwGirfuNh0lQVrI6AESXy4JhptODZLHgDgkfmI7MzEiSQ1CCW45JuRAs15LHyMPoGRcGhVComDu9u37zl5ZaZUWA7QQcG2y2rWrTAUM2ucDulw05WuuVaQN0P6fdz4XmCTP1euobxFZsElFyyn0l/a8pS0m96skTWTf+9v84xG6B/N7zdLK0f1wr7gR+dXSeXVYw+yJmHpasIUA4/ic0gATWGODh7XgVdhR/EsWswKg3QYM89DzKhW2BGO/rgRsWqFWioAY4/RxDhEG4SIbrtEQ9jmtC0UGNA07lJ5UOEOl/8rX6tnPF+LN5lgQdKinOZJbEgvTTI/hK+s2x8/nJNJSfAeBY6rZOeKtg6AbxgJpBpWB5O3OqGpd7QSB4Eer7UNZKX3pN9SWZm8YCTUQaCzWwwogzmfd8L5gSIOD3ZGvpJs5gJgFKwe9ISP4WWTbmVpmKkqf8DoFwgtMqOwplicQfTMxigZkcGczGhIcY5SjeRnY0Zpkn4WA5m0Lnlegf+n1ECRR+luC8pQoaF1l8oZo70PVFmsNycRGaqQ+cvoDsdJaHC/Q0Q/6dAk2lCa94eFl/aH5Td0anLNR7BdF+rGPaBbOoLxJAXIJ31x6LEc8eqFt0RtFsaXXjO1m9Deb5LrlXFt8mCzy0eXpmniUhAhpQnNZRHjGRAz1r7s7/+BRrKAYBE5QFuVp0f7djlDEz3SLpvCJqo+4yoTZSlAtraOSvFWIX1SFXwupaRlSZZuQHwRkrpRIbCjISh4+UW5E1V2JEC18pAwA7wozyqGoftcJhOsyB7a2wQBmKlCsRZlmci2CM4S7DsJklp9jchI+hNjvQPnXQFoL1VsNbTe64NyOm3aI89REcCistbpDRzTGIQG81MOmGNyt9qA2GbZGaQbHTFx1U07taslsYzrjqyq/MC+7NzjLu7VoG7CT9QT6vWXYGiS/dofwK2GHBrtn9meZleyqj9mqnJo+LdVYA5SSZuTwBx6Ad9TxUvTiknTIIS0Cr1KWFrD9eh2UVrUf+P13Ha5lrbX2Zv+l0KNKtoTv36UIXzeFKGUsf5wBtqxO9AdkNJxwZOWnKJhg1ma2jT9QuhZMR7rJTO4KydAsKbxyb59wHrEpZ9wG4FhuYZtqbMHRr6FRxlQLXis/7FJQlc/bOH5CzbfTW/F+yCtKpbDpDtzKxAiuBzS+NQSYOkAbO2QI8wOaJx/ny6cBSbNdUODOYk63PJXt7Ah7uIPrPM7Z64pxUfk8J9S3Y49mNJGWLGnr7bzM+liJKx9tf5q0NbE/goMMMlVhcvAlrzTkPideMa9+oGaa+Us1esACihYhwzBbslDJCMIY+qwjhTHvtBuU834gtHtiGmhWsZ2MnHTTdPBIll6jbL0d6JDrM3urkUif8W8VCZn4ey6l5YcoblFdKpvmMLRhqYVFb6UIShPF8b6XwR9XP26LvLM5K+RiR7r3mgN3/yEj/w8+H9y8mRrl2sh/oM8kjK3DIYxe6f2OBG0TsSkJboiZr1/p5Jf7axYgjrSiJbZJ25PJfIDDLO/0lu4OJ86CK1Ean3llKAZRG3yk8/1/rjrIj5w2E/HMfqkwfwT5Y319G45j5Lzhylaq7UlNSQ9wHPnLOHrWLbuDdNPpHoWAg0j1vDobsJu6omK7yzPGE+HVMS7qKUKG/pkMB8OWAng/W/r12GnmaYmIc6V/AMkFT7PPEeRX5gsgMMIIJwQYJKoZIhvcNAQcBoIIJsgSCCa4wggmqMIIJpgYLKoZIhvcNAQwKAQKgggluMIIJajAcBgoqhkiG9w0BDAEDMA4ECFVMr1ootfavAgIIAASCCUhu+JGfEHHuqI3AbsbJJ9r9RkcrqRrGkFGwMrb/ylEpscXCpFdCRLkLRYbwxpsLTlHY6wFT1Vxrueav2Yj4PyPyWQ2Hw78KjB5qZtcBd+cxGSJLRom3meZfNtO7JXVyi3Xw8fnS3E8rYQwdRfeKvhAKKh/+ak31e9ZVACjxzLqB/SinZjPDk7T7P4edZdC92d/OF2sQaU72B4vsgCONHTt53UBmpBTfsJI1o97Op9umKiJFMua71mpTebcNuPrp2i8OfrFqsZ4ZsUVK5HXo8ulaYBSZzUZQkWUhLR6hpSpArkRLxoypJQs4QA3io8S4rCLVEZFeQlDGcqYhNn4xUgNHBn8WYb2MUFzRtUuJxf1qknB6gsBpwhFCcUL8ck+O6IS53Z/kegxuBIaYymIi+H2oqXNWZGeB07pm0qdStdEjKYSIj49Xppv+NIJHSF4dgEzJhKXk+bkZdtydZp0NcB2pYzFmMDXV0KXbW9BxjWxluC9HziCVW3RHzmFX0CRc9mAP4JlsRPhZgk0ez544/HwvcGCnEeFoDHJkJTlcocqOFSivQP5t2VoWPRKx1rbfdn9KfkFMLZ8yvXlPgBlY9WPPe+N4V6RgbfWiygRQQ8G/9awoNbY0sgaolH9MRsivsVZWxuTg/dzDLCwvZX9jvT4XqpFMKS+dLDEEf+LOI8Vd+faaR/JU9mtkTBsXkeuglIT69MRkqYVVKP68xSWIBGsI8rRWU6Kk82qlm7lGMLfSpXXRV1ghZWi/x++1exZm3mH9jRauw9pEq6ZPmz48vOJMEuiUopHootXQOfUcaaXreh+UpUG4n5tx/Srj6C/kvBRb5uBNqUCMabOwRz9jZ1ULspQypdFKfyVypfUonbRSQHCCaQ0qlQRfUNbgTMno3eNRxQHUGYB0BrmMpxNm05vG9XdmzqxxW1DAU3ykWQqTK+ToBOoGLxN58+I6IkbHogq8VcYx7DRe9JyCeEsudvxFG6QXGHjcHH1FLaUJPUD6j/NRR2TejfeA1qW35lzrpMrqmwVO8GACsIOl23K0Wday60PNk8aE+e5fqhDiaHCyyFpEngw81b5tNb4LY6q+zguVBjhdjC/oC7okoouQw9Ggtyz/g4xZZEDaSvAP2Rccc1GmJsJFgWCy57eS9aO7YXgjgx/+A5OhwHkiGAtzNzdnCfeTeD0ologbF667iQOVoaB+uWYi+g6pPsq5jafkRwyl5NT8jl53TCJbYzH0NiB+i3fS1v2ulpv6OM7nEXd0H9RCy7cBnifNvKh8txhd+jpVaohkq0+NI+ZjLCTKGtrYPfQFStG8g5YrroTfmttdRYBVcAbbcFKpkgte90DFgsB7Wo42T0+ltrhJVWA7YowYx3HZVoJObF/OKfQoqOIARn8R/6qhgy4BxxFEWR1WRk/w7/Vi+q//PpoGSC4R/c0TX2LRzmNOT+BvIAm6Q1TLxnpEWAm23Nt709IsfET50NyUU16WLtuX+dNWIEkMupmcPmbG5RM/qoZd4jTZcUbKC7EIQiefuGe0ecdmr3JE9kCdJILQNQkViMsoDz+84KaYa6rfYsgQjMYapmCJd2CRQUnCu9Jxr8ZwSs6XW/GM4/PxtERMg1B1YQDooSLc/9c+usx29NVHyzHlLRdSv7tFAGefjN/zf1CVhLIF3YbQDdt3EJFO+lcl7ebE90TlZ5fIEmXVcs+9k0v4utxRKnHmCyq+8xVFXJucMNu62FX6jhYadxF4sC9bZb1a1hg3uN49RebzPeuHtRgb5Rk+2rRKrJKVk6PwYoJGs1Ttd/xAySt9Wk4mKVxHwOspFxMsKZZx0CAkN5oJS9jAqEwX0jMRXFSwx85dHw6PNIYHwiuN+F+5oK4sEUK7KZHeXYTANTZlavM+SVH4IK3vZFhhi37QfrZr1Ph6NERLssZ8NmfXgC4l0IJFVqu+U3ilMaodyeIrOeFCr9qHafwxYZzdr2+umla1EzgxtZsgpqFdegAxa0XRTvfv9fmujzoPHw/1uIRIEywzzWFaCPqMG+c+A4FswEMQEDInBmYFyPQ4FUAKy30yarTJFch8bc1pl3EY1k3vuOP8dAWnAH4fG79u3NVMAz0E3Ew2h/qAQocpUJu7ZOpKEJCZYL6ha2RjZ5TmJWR51bmjujtpAGGNRYOl+1hUacf0DuIoxDz37/uYWx5nfylRcsDeQo5x5Uj/oQ1KzNT+r6OKHvgeYzhbzdf9KqOUNMFiLfrEPoB6g04sELQFXSOTf6tZG7PXbjkFo4+IKIFyOzf+DBF3f510+AogCcJT3KCju3CHolVFvrLqi+wvK540WcbQWnMOu5VQseHw6Y/77GiFaIaO12bIctlAkiBwVn67j5pEdq56z2TSxvfAYOGKGeHOcXaKiY3opaMu9piQTf4Qkoo81P08SW02rIfAV/8c6MVDT9OlaSdVa7L65m0QAjr8hMxBECou6DgWnpYE5jnL5Y/voBTUwFP7bfghwSU90VvnPgAJS/+Tquk5qbNEtu9X6td4kQizcp9ACCUqVSgwiDVyq0iW9Le5F5CIM3MmS7A3PlMTo4av4hcMmWNaOXsgADlghNzsIVeIZyVRDTHIKUeHeWwd9gvkfn33cZcjK5rq3e5autm1acQlHuxFBBlU614kfOzZ9LyJUICeDkcC4ALcpA9sRVV9iHmsDCzWWn7cOkZN8ssBDMTsLSmdN9DlWPeZIzHVXLyynEpNJXAg+rHbZwlX/UNlM+zzrMNQSmJ9KO2MvRaiN3opFNpuG8n0ziFX/VcWIbNkH4Ip/q0e5PIVoKv6MjDdoid9gqBu6uUjIXTXdD/KvGgFzbIYEGO7yQEjfG2l45+9RsOaCNo/dsq73haZagTP2D7GSZV2S4U2by5ECS+Wy186/xaDG1rnq+vGpGL2Fuuze2TUrlfYUnvae1eJU7VyaCgEksyue30jRfUlJzRI1a1+8uV9LmlX8LmVtZci2fAgffLqijyNyXQ00SknS4zyFWO+CZ27VEVOVoZCbe0zKztlErnLqxkxsxClmPGnoYHgExPQYdFMLapoUTWRyiScJHL3hUn8kcXjQQlqSrFgTszhNgWv3ThxazESeSkGsBUJYlo4Q8UQPvkaGzk2lQZ2urXaX+w5VcrOafCmhqey4ABVSpzsw12QIvzaztGRwOgrgOTzMzWe9OdSP60xJTAjBgkqhkiG9w0BCRUxFgQU1HeGJHXuTJv45JicZz/LonXL/sQwKzAfMAcGBSsOAwIaBBQiWtjvU/7ZkC0l5zYrRE4e73Ar2AQIogEP7IsTZ5M=")
	truststoreBytes, _ = base64.StdEncoding.DecodeString("MIIGNgIBAzCCBgIGCSqGSIb3DQEHAaCCBfMEggXvMIIF6zCCBecGCSqGSIb3DQEHBqCCBdgwggXUAgEAMIIFzQYJKoZIhvcNAQcBMBwGCiqGSIb3DQEMAQYwDgQILQ1Xx4EyxeICAggAgIIFoHgpah53vxSlG/4VGlZpKyAU3sbp7bjmjKpMx5+LRdeF09l5VF92Xmn4kP8uDLfsqoFDg1ASeaQGvkREY0o0FWgbbHBYpjVPfaWS35QaM2pWl+R5ih3E4SokRtx5NqF6zNQtU27NZSAzHaTDIlE/jsQs6ZtwEULHGFzL9/UQK9QlMIekURo1RoNMvQHwCn9Cg2ArYuqliYLJdhvpy39rzBrl+PwGgGN4aOspSs9LA6WmK6V7Kxg/jaswve5g7Zf5xi4Zd2njn5GsYyG9CscegF4DpLaFvlOMKFkJiOZ4fv/2fA8BhPC7umUuyKggKwkJk/ZMeMKfd7S0A5EEW64LO+9KzWZ+wYwo781bvMivqV8zStTHZzzvebmTaS/mfB+PFNbUVQpxhynM7xRT83qCN2+2GZ3JbfPkyeA+T4LClwPG/M1Na9o2aCBZ0YNwoDvkNjo+GEIk+v52/giMTw2ZLvaWPh1OxGf37BsjabBHKgDen7L82bHe9HK28WMvJ4Oh5fBoLzNLmTp+u4UvGix3LN65YDNekfv0djUbOeSIqJk8mXXoMd3uxPF/bvJjVMkt7ZqLmPEZ3EvCcmrLoOxGv64GJ1sNoomeEu2MCJxCOdR1CAufoExuwoLdoAo5C7Ja2VorusgWd72Ii9B1V33S2zM6W6VE2kbMuE9lQamLt1KquCQTHE2/5eOc6S6ooV8175fp5gm/T61UaMsKlKGuP7ENlbx5EbgGd1qabe9J0aNrGWcnWvqQxJ/chAA0fdYH16tFPljpMplVTKYVWfjJkicWLS8UR7NahGop8tNIPGy+1fc3OCsxuNaDewaLt9OAKOxRY5soL7X1A6gnd+DJih+eJf76i0Wn1L4dLrnzqaAMGDxYQ3ytxmTN/mzfoA6MjioR8E13R0WyIdptjlNZogEzwb9/zRvoa/aDAvOQQ9bvsQxk2PrqL0xDvxiYSSikLB1N8ao0R8vpXxHNryV0j6W08S+7L46hc5tysi/oH4o3E8LSfBMShIAGJZrnmZ3A4WHGRx7hOczbsnTOZtDLrdqgJtH/zrfB64m6QKcRZ5qqO7qjEo/cAwutLM9GfmOx2Ht4E8St5kbXIlaFwIC3IUgnja+uK90OzrrhW/fQ2ToJj6ZKJHsSwduWzQAuCx/NoonfhfFErCF9d104fsqZTmklA7zr+dq8+CPGPWCAEUavtwVhKJjDFPb02ASNnfZo/my+1EZ9dwdB/qq56t3TdDpVDImFInGuf06kkW1I8B9G0PC+s0xrYM7A42s/74yrJ8ISxUnfQlQiFGX5uzUswsHWdh9fV1K2aT6CfaF6Fg6M+OqPTwrdlYK1bZWq9Ome5wJ2l/5ZElcSXng7nTGzSjebwkD63O1csw1vekvYYTfrvuMKvXVhm2WxSj2vIINxX/SgJW8BEVMSac26hxJGuoa/Tg3isZQkwIJ3kiyxMBENvpoZb7Unbyvz+BFerKZdb5E/k25Uj4G6nv5FeNY6Tvl4wKGvMdd2fjPWGjACjvSCPQSdKzZgdUYmUffkAZx9HIg9KclxuqMc/FA0r8wgnR0VdWevrBIl7m6X9WymkCA6lkJxFOetHI7h3g41yVlomQsqOoJ+rQAMaQVYXUjEIFh0ZgBg1hPMDb2AEMWkvDhd055XjDA8AAETqebJ0Olqt7i60fT5udoA1zu45yTHh06M3Kjrmg8j94IPaBLbt/d+6ohj3tSvVv9Z7XYXxetge7IFGdqboA1V/96O9VrLNE7rvZqpFo0h23FJ0BS57+lnlThpybzIS6Xj5/bSDntc4RoWlpexAUZQqJ9TIkNjwdYj2Z5U5XyDpG9krne2QWh6QO2+tu1l3t4SQd9Aj5KAaYrOIG/ekFsXRcHFIVuOUFdBANCIdnNNejm6xenOR58kKdJHGtmcsPsejT38rJVQJDArMB8wBwYFKw4DAhoEFHVP5fQwiYdT2bDhDXGJ0Yu52w8OBAhcpcLa2OOwhA==")
	keystorePass       = "cassandra"
)

func mockedTLSNodeSecret(cc *v1alpha1.CassandraCluster, name string) *v1.Secret {
	nodeTLSSecretData := make(map[string][]byte)
	nodeTLSSecretData["ca.crt"] = caCrtBytes
	nodeTLSSecretData["tls.crt"] = tlsCrtBytes
	nodeTLSSecretData["tls.key"] = tlsKeyBytes
	nodeTLSSecretData["keystore.p12"] = keystoreBytes
	nodeTLSSecretData["truststore.p12"] = truststoreBytes
	nodeTLSSecretData["keystore.password"] = []byte(keystorePass)
	nodeTLSSecretData["truststore.password"] = []byte(keystorePass)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cc.Namespace,
		},
		Data: nodeTLSSecretData,
		Type: v1.SecretTypeOpaque,
	}
}

func TestValidateTLSFields(t *testing.T) {
	nodeTLSSecret := mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret")
	asserts := NewGomegaWithT(t)
	k8sResources := []client.Object{
		baseCC,
		nodeTLSSecret,
	}
	tc := test{
		name:         "should validate a good cluster node secret",
		cc:           baseCC,
		errorMatcher: BeNil(),
		params: map[string]interface{}{
			"cluster-node-tls-secret": v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: "test-cluster-node-tls-secret",
				},
			},
		},
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
		reconciler.Client = tClient
		reconciler.defaultNodeTLSKeys(&tc.cc.Spec.Encryption.Server.NodeTLSSecret)
		err := reconciler.validateTLSFields(tc.cc, nodeTLSSecret, serverNode)
		asserts.Expect(err).To(tc.errorMatcher)
	})
	nodeTLSSecret = mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret")
	delete(nodeTLSSecret.Data, "ca.crt")
	tc = test{
		name:         "should error on a bad cluster node secret",
		cc:           baseCC,
		errorMatcher: Not(BeNil()),
		params: map[string]interface{}{
			"cluster-node-tls-secret": v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: "test-cluster-node-tls-secret",
				},
			},
		},
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
		reconciler.Client = tClient

		reconciler.defaultServerTLS(tc.cc)
		reconciler.defaultClientTLS(tc.cc)
		err := reconciler.validateTLSFields(tc.cc, nodeTLSSecret, serverNode)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(err.Error()).To(Equal("TLS Secret `test-cluster-node-tls-secret` has some empty or missing fields: [ca.crt]"))
	})
}

func TestReconcileNodeTLSSecret(t *testing.T) {
	asserts := NewGomegaWithT(t)
	k8sResources := []client.Object{
		baseCC,
		mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret"),
	}
	tc := test{
		name:         "should reconcile a good cluster node secret",
		cc:           baseCC,
		errorMatcher: BeNil(),
		params: map[string]interface{}{
			"cluster-node-tls-secret": v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: "test-cluster-node-tls-secret",
				},
			},
		},
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
		reconciler.Client = tClient

		restartChecksum := checksumContainer{}
		reconciler.defaultServerTLS(tc.cc)
		reconciler.defaultClientTLS(tc.cc)
		secret, err := reconciler.reconcileNodeTLSSecret(context.Background(), tc.cc, restartChecksum, serverNode)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(secret).ShouldNot(BeNil())
		asserts.Expect(secret.ObjectMeta.Annotations).Should(HaveKeyWithValue("cassandra-cluster-instance", "test"))
	})

	secret := mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret")
	secret.ObjectMeta.Annotations = map[string]string{"cassandra-cluster-instance": "doh", "fizz": "pop"}
	k8sResources = []client.Object{
		baseCC,
		secret,
	}
	t.Run(tc.name+" with annotations", func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
		reconciler.Client = tClient

		restartChecksum := checksumContainer{}
		reconciler.defaultServerTLS(tc.cc)
		reconciler.defaultClientTLS(tc.cc)
		secret, err := reconciler.reconcileNodeTLSSecret(context.Background(), tc.cc, restartChecksum, serverNode)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(secret).ShouldNot(BeNil())
		asserts.Expect(secret.ObjectMeta.Annotations).Should(HaveKeyWithValue("cassandra-cluster-instance", "test"))
		asserts.Expect(secret.ObjectMeta.Annotations).Should(HaveKeyWithValue("fizz", "pop"))
	})

	tc.name = "should invalidate secrets"
	tc.errorMatcher = Not(BeNil())
	errMsg := "failed to validate encryption.server.nodeTLSSecret.name: test-cluster-node-tls-secret fields: TLS Secret `test-cluster-node-tls-secret` has some empty or missing fields: [%s]"

	requiredFields, _ := newTLSSecretRequiredFields(tc.cc, serverNode)
	for _, key := range requiredFields {
		secret := mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret")
		delete(secret.Data, key)
		k8sResources = []client.Object{
			baseCC,
			secret,
		}

		t.Run(fmt.Sprintf("%s without %s", tc.name, key), func(t *testing.T) {
			reconciler := initializeReconciler(tc.cc)
			tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
			tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
			reconciler.Client = tClient

			restartChecksum := checksumContainer{}
			reconciler.defaultServerTLS(tc.cc)
			reconciler.defaultClientTLS(tc.cc)
			secret, err := reconciler.reconcileNodeTLSSecret(context.Background(), tc.cc, restartChecksum, serverNode)
			asserts.Expect(err).To(tc.errorMatcher)
			asserts.Expect(secret).To(BeNil())
			if err != nil {
				asserts.Expect(err.Error()).To(Equal(fmt.Sprintf(errMsg, key)))
			}
		})
	}

	tc.name = "should validate a good client secret"
	tc.params = map[string]interface{}{
		"client-node-tls-secret": v1alpha1.ClientEncryption{
			Enabled: true,
			NodeTLSSecret: v1alpha1.NodeTLSSecret{
				Name: "test-client-node-tls-secret",
			},
		},
	}
	tc.errorMatcher = BeNil()
	k8sResources = []client.Object{
		baseCC,
		mockedTLSNodeSecret(baseCC, "test-client-node-tls-secret"),
	}

	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Client = tc.params["client-node-tls-secret"].(v1alpha1.ClientEncryption)
		reconciler.Client = tClient

		restartChecksum := checksumContainer{}
		reconciler.defaultServerTLS(tc.cc)
		reconciler.defaultClientTLS(tc.cc)
		secret, err := reconciler.reconcileNodeTLSSecret(context.Background(), tc.cc, restartChecksum, clientNode)
		asserts.Expect(err).To(tc.errorMatcher)
		asserts.Expect(secret).ShouldNot(BeNil())
		asserts.Expect(secret.ObjectMeta.Annotations).Should(HaveKeyWithValue("cassandra-cluster-instance", "test"))
	})
}

func TestReconcileTLSSecrets(t *testing.T) {
	asserts := NewGomegaWithT(t)
	k8sResources := []client.Object{
		baseCC,
		mockedTLSNodeSecret(baseCC, "test-cluster-node-tls-secret"),
		mockedTLSNodeSecret(baseCC, "test-client-node-tls-secret"),
	}
	tc := test{
		name:         "should reconcile cluster and client secrets",
		cc:           baseCC,
		errorMatcher: BeNil(),
		params: map[string]interface{}{
			"cluster-node-tls-secret": v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: "test-cluster-node-tls-secret",
				},
			},
			"client-node-tls-secret": v1alpha1.ClientEncryption{
				Enabled: true,
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: "test-client-node-tls-secret",
				},
			},
		},
	}
	t.Run(tc.name, func(t *testing.T) {
		reconciler := initializeReconciler(tc.cc)
		tClient := fake.NewClientBuilder().WithScheme(baseScheme).WithObjects(k8sResources...).Build()
		tc.cc.Spec.Encryption.Server = tc.params["cluster-node-tls-secret"].(v1alpha1.ServerEncryption)
		tc.cc.Spec.Encryption.Client = tc.params["client-node-tls-secret"].(v1alpha1.ClientEncryption)
		reconciler.Client = tClient
		reconciler.defaultServerTLS(tc.cc)
		reconciler.defaultClientTLS(tc.cc)
		err := reconciler.reconcileTLSSecrets(context.Background(), tc.cc)
		asserts.Expect(err).To(tc.errorMatcher)
	})
}
