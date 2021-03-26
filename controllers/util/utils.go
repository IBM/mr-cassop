package util

import v1 "k8s.io/api/core/v1"

func MergeMap(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func GetNodeIP(addressType v1.NodeAddressType, nodeAddresses []v1.NodeAddress) string {
	for _, addr := range nodeAddresses {
		if addr.Type == addressType {
			return addr.Address
		}
	}
	return ""
}
