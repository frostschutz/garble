// garble produces pseudo random bytes based on a phrase
// and uses it to garble and ungarble files
package main

// #include <stdint.h>
// #define BSIZE 65536
//
// void xor(int64_t *a, int64_t *b) {
//     int i = BSIZE / 8;
//     while(i--) {
//         a[i] ^= b[i];
//     }
// }
import "C"
import "unsafe"

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"bitbucket.org/MaVo159/rand"
)

const (
	BSIZE   = C.BSIZE
	MULTI   = 4
	SOURCES = 8
	POOL    = SOURCES * (MULTI + 1)
)

var (
	narg   = int(0)
	stdout = false
	phrase = ""
	pool   = make(chan []byte, POOL)
	loop   = make(chan []byte, POOL)
	data   = make([]chan []byte, SOURCES)
)

// randomSeed produces a int64 seed based on crypto/rand and time.
func randomSeed() uint64 {
	var seed uint64

	urandom := make([]byte, 8)
	cryptorand.Reader.Read(urandom)

	for key, value := range urandom {
		seed ^= (uint64(value) ^ uint64(time.Now().UTC().UnixNano())) << (uint(key) * 8)
	}

	return seed
}

// randomBytes fills byte buffers with random data
func randomBytes(src rand.Source, out chan<- []byte) {
	var (
		r uint64
		i = BSIZE
	)

	for buf := range pool {
		for i = 0; i < BSIZE; i += 8 {
			r = src.Uint64()
			buf[i] = byte(r)
			buf[i+1] = byte(r >> 8)
			buf[i+2] = byte(r >> 16)
			buf[i+3] = byte(r >> 24)
			buf[i+4] = byte(r >> 32)
			buf[i+5] = byte(r >> 40)
			buf[i+6] = byte(r >> 48)
			buf[i+7] = byte(r >> 56)
		}

		out <- buf
	}
}

// xor a file with random data
func garble(fin *os.File, fout *os.File, in <-chan []byte, out chan<- bool) {
	var n, m int
	var err error
	data := make([]byte, BSIZE)
	var buf []byte

	for ok := true; ok; {
		// read
		n, err = fin.Read(data)
		for n != BSIZE || err != nil {
			if err != nil && err != io.EOF {
				panic(err)
			}
			if n == 0 {
				goto final
			}
			m, err = fin.Read(data[n:BSIZE])
			if m == 0 && err == io.EOF {
				// last partial block
				ok = false
				break
			}
			n += m
		}

		// xor with random data
		buf = <-in
		C.xor((*C.int64_t)(unsafe.Pointer(&data[0])), (*C.int64_t)(unsafe.Pointer(&buf[0])))
		out <- true // done with buf

		// write
		_, err = fout.Write(data[0:n])
		if err != nil {
			if err2, ok := err.(*os.PathError); ok && err2.Err == syscall.EPIPE {
				goto final
			}

			panic(err)
		}
	}

final:
	close(out)
	for {
		<-in // sleep forever
	}
	return
}

// parse command line arguments
var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func init() {
	flag.StringVar(&phrase, "phrase", "", "the Garble phrase, by default random")
	flag.Parse()

	narg = flag.NArg()

	if narg <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	if phrase == "" {
		phrase = fmt.Sprintf("%016x", randomSeed())
	}

	fmt.Println("Using phrase:", phrase)
}

// the main program...
func main() {
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Use available CPUs:
	if runtime.GOMAXPROCS(0) == 1 &&
		runtime.NumCPU() > 1 &&
		os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Open files:
	writers := make([]chan []byte, narg)
	signals := make([]chan bool, narg)

	for i, arg := range flag.Args() {
		var fd [2]*os.File
		for i, _ := range fd {
			if !stdout && arg == "-" {
				fd[0] = os.Stdin
				fd[1] = os.Stdout
				stdout = true
				break
			}
			f, err := os.OpenFile(arg, os.O_RDWR, 0666)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			fd[i] = f
		}

		writers[i] = make(chan []byte, MULTI)
		signals[i] = make(chan bool, MULTI)
		go garble(fd[0], fd[1], writers[i], signals[i])
	}

	// Allocate byte buffer pool:
	buffer := make([]byte, BSIZE*POOL)

	for i := 0; i < BSIZE*POOL; i += BSIZE {
		pool <- buffer[i : i+BSIZE]
	}

	// Initialize random sources:
	hash := sha512.New()
	sum := make([]byte, hash.Size())

	for i := 0; i < SOURCES; i++ {
		var seed, s uint64
		var err error

		hash.Write([]byte(":garble:" + phrase))
		hash.Sum(sum[:0])

		buf := bytes.NewReader(sum)
		s = 0

		for err == nil {
			err = binary.Read(buf, binary.LittleEndian, &s)
			seed ^= s
		}

		src := rand.NewSource(seed)
		data[i] = make(chan []byte, MULTI)
		go randomBytes(src, data[i])
	}

	// Route data channels:
	go func(data []chan []byte, writers []chan []byte, signals []chan bool) {
		var buf []byte
		for {
			for _, r := range data {
				buf = <-r

				for i, w := range writers {
					if signals[i] != nil {
						w <- buf
					}
				}

				loop <- buf
			}
		}
	}(data, writers, signals)

	var buf []byte

	for narg > 0 {
		buf = <-loop

		for i, s := range signals {
			if s != nil {
				_, ok := <-s

				if !ok {
					signals[i] = nil
					narg--
				}
			}
		}

		pool <- buf
	}

	fmt.Println("All done!")
}
