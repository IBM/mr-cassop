package util

import (
	v1 "k8s.io/api/core/v1"
	"math/rand"
	"time"
)

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

func GenerateAdminPassword() string {
	rand.Seed(time.Now().UnixNano())
	digits := "0123456789"
	specials := "_" // Todo: add validation for generated password to deal with reaper password read issue

	allChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" + "abcdefghijklmnopqrstuvwxyz" + digits + specials
	length := 16
	buf := make([]byte, length)
	buf[0] = digits[rand.Intn(len(digits))]
	buf[1] = specials[rand.Intn(len(specials))]
	for i := 2; i < length; i++ {
		buf[i] = allChars[rand.Intn(len(allChars))]
	}
	rand.Shuffle(len(buf), func(i, j int) {
		buf[i], buf[j] = buf[j], buf[i]
	})
	return string(buf)
}
