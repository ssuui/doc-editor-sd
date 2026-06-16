package auth

import (
	"crypto/rand"
	"math/big"
	"time"
)

const tokenCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func GenerateToken() string {
	out := make([]byte, 32)
	for i := range out {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(tokenCharset))))
		out[i] = tokenCharset[n.Int64()]
	}
	return string(out)
}

func CalcExpireTs(hours int64) int64 {
	return time.Now().Add(time.Duration(hours) * time.Hour).Unix()
}
