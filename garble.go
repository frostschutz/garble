package main

import (
	"crypto/rand"
	"fmt"
	"time"
)

// randomSeed produces a int64 seed based on crypto/rand and time.
func randomSeed() int64 {
	var seed int64

	urandom := make([]byte, 8)
	rand.Reader.Read(urandom)

	for key, value := range urandom {
		seed ^= (int64(value) ^ time.Now().UTC().UnixNano()) << (uint(key) * 8)
	}

	return seed
}

func main() {
	for i := 0; i < 100000; i++ {
		fmt.Println(randomSeed())
	}
}
