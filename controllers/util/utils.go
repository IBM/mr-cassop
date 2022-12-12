package util

import (
	"crypto/sha1"
	"fmt"
	"math/rand"
	"time"

	v1 "k8s.io/api/core/v1"
)

func MergeMap(a, b map[string]string) map[string]string {
	if a == nil && b == nil {
		return nil
	}
	res := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		res[k] = v
	}
	for k, v := range b {
		res[k] = v
	}
	return res
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

func Sha1(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	bs := h.Sum(nil)
	// Use the %x format verb to convert a hash results to a hex string
	return fmt.Sprintf("%x", bs)
}

// EmptySecretFields returns empty fields in Secret based on input field list
func EmptySecretFields(secret *v1.Secret, fields []string) []string {
	var emptyFields []string

	if secret.Data == nil {
		return fields
	}

	for _, field := range fields {
		if len(secret.Data[field]) == 0 {
			emptyFields = append(emptyFields, field)
		}
	}
	return emptyFields
}

func Uniq(s []string) []string {
	var result []string
	inResult := make(map[string]bool)

	for _, str := range s {
		if _, ok := inResult[str]; !ok {
			inResult[str] = true
			result = append(result, str)
		}
	}
	return result
}
