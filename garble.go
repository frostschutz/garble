package main

import (
	cryptorand "crypto/rand"
	"crypto/sha512"
	"flag"
	"fmt"
	//	"math/rand"
	"os"
	"runtime"
	"time"
)

var (
	phrase = ""
)

// randomSeed produces a int64 seed based on crypto/rand and time.
func randomSeed() int64 {
	var seed int64

	urandom := make([]byte, 8)
	cryptorand.Reader.Read(urandom)

	for key, value := range urandom {
		seed ^= (int64(value) ^ time.Now().UTC().UnixNano()) << (uint(key) * 8)
	}

	return seed
}

func init() {
	flag.StringVar(&phrase, "phrase", "", "the Garble phrase, by default random")
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if phrase == "" {
		phrase = fmt.Sprintf("%016x", uint64(randomSeed()))
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	args := flag.Args()

	for i := range args {
		fmt.Println(args[i], "with", phrase)
	}

	hash := sha512.New()
	seed := []byte(phrase)
	b := make([]byte, hash.Size())
	for i := 0; i < 2; i++ {
		hash.Write(seed)
		hash.Sum(b[:0])
		fmt.Println(b)
	}
}
