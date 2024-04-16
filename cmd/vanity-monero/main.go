package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	vanity "ekyu.moe/vanity-monero"
	"ekyu.moe/vanity-monero/mnemonic"
)

type (
	workMode  uint8
	matchMode uint8
)

const (
	wmStandard workMode = iota
	wmSplitKey

	mmPrefix2 matchMode = iota
	mmPrefix
	mmRegex
)

var stdin = bufio.NewScanner(os.Stdin)

func main() {
	var wMode = wmStandard 

	var dict = mnemonic.English

	var mMode matchMode
	var initIndex int
	fmt.Println(`Welcome to Morelo vanity generator.
This program allows You to generate Morelo wallet with custom prefix,
or using regex pattern.`)
	fmt.Println()
	fmt.Println("To start please select match mode:")
	fmt.Println(`1) Prefix from the 5rd character. (fast)
   For example, pattern "Ai" matches "emo1Aiabc...".
2) Regex. (slow)
   For example, pattern ".*A[0-9]{1,3}i.+" matches "emo1abcA233idef...".
   Note that in Regex mode there is no guarantee that there exists such address matching the pattern.`)
	switch promptNumber("Your choice:", 1, 2) {
	case 1:
		mMode = mmPrefix2
		initIndex = 4
	case 2:
		mMode = mmRegex
	}
	fmt.Println()
	
	var network = vanity.MoreloMainNetwork
	fmt.Println("Select network:")
	fmt.Println("1) Morelo MAINNET")
	fmt.Println("2) Morelo TESTNET")
	fmt.Println("3) Morelo STAGENET")
	switch promptNumber("Your choice:", 1, 3) {
	case 1:
		network = vanity.MoreloMainNetwork
	case 2:
		network = vanity.MoreloTestNetwork
	case 3:
		network = vanity.MoreloStageNetwork
	}
	fmt.Println()

PATTERN:
	var regex *regexp.Regexp
	var needOnlySpendKey bool
	pattern := prompt("Enter your prefix/regex, which must be in ASCII and not include 'I', 'O', 'l':")
	switch mMode {
	case mmPrefix2:
		if !vanity.IsValidPrefix(pattern, network, 4) {
			fmt.Println("invalid prefix")
			goto PATTERN
		}
		needOnlySpendKey = vanity.NeedOnlySpendKey(pattern)
	case mmRegex:
		var err error
		regex, err = regexp.Compile(pattern)
		if err != nil {
			fmt.Println("invalid regex:", err)
			goto PATTERN
		}
		needOnlySpendKey = false
	}

	caseSensitive := true
	if strings.ToLower(prompt("Case sensitive? [Y/n]")) == "n" {
		caseSensitive = false
		if mMode == mmRegex {
			regex = regexp.MustCompile("(?i)" + pattern)
		} else {
			pattern = strings.ToLower(pattern)
		}
	}

	n := promptNumber("Specify how many threads to run. 0 means all CPUs:", 0, 65535)
	runtime.GOMAXPROCS(n)
	threads := runtime.GOMAXPROCS(0)
	fmt.Println("=========================================")

	diff := uint64(0)
	switch mMode {
	case mmPrefix2:
		diff = vanity.EstimatedDifficulty(pattern, caseSensitive, false)
	}
	if diff == 0 {
		fmt.Println("Difficulty (est.): unknown")
	} else {
		fmt.Println("Difficulty (est.):", diff)
	}
	fmt.Println("Threads:", threads)
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan *vanity.Key)
	ops := uint64(0)
	for i := 0; i < threads; i++ {
		// more code but less branches
		if wMode == wmStandard {
			if mMode == mmRegex {
				go func() {
					seed := new([32]byte)
					key, addr := &vanity.Key{}, ""
					for ctx.Err() == nil {
						rand.Read(seed[:])

						key = vanity.KeyFromSeed(seed)
						addr = key.Address(network)

						if regex.MatchString(addr) {
							cancel()
							result <- key
							return
						}

						atomic.AddUint64(&ops, 1)
					}
				}()
			} else if needOnlySpendKey {
				if caseSensitive {
					go func() {
						seed := new([32]byte)
						key, addr := &vanity.Key{}, ""
						for ctx.Err() == nil {
							rand.Read(seed[:])

							key = vanity.HalfKeyFromSeed(seed)
							addr = key.HalfAddress(network)

							if strings.HasPrefix(addr[initIndex:], pattern) {
								cancel()
								result <- key
								return
							}

							atomic.AddUint64(&ops, 1)
						}
					}()
				} else {
					go func() {
						seed := new([32]byte)
						key, addr := &vanity.Key{}, ""
						for ctx.Err() == nil {
							rand.Read(seed[:])

							key = vanity.HalfKeyFromSeed(seed)
							addr = strings.ToLower(key.HalfAddress(network))

							if strings.HasPrefix(addr[initIndex:], pattern) {
								cancel()
								result <- key
								return
							}

							atomic.AddUint64(&ops, 1)
						}
					}()
				}
			} else { // Need full key
				if caseSensitive {
					go func() {
						seed := new([32]byte)
						key, addr := &vanity.Key{}, ""
						for ctx.Err() == nil {
							rand.Read(seed[:])

							key = vanity.KeyFromSeed(seed)
							addr = key.Address(network)

							if strings.HasPrefix(addr[initIndex:], pattern) {
								cancel()
								result <- key
								return
							}

							atomic.AddUint64(&ops, 1)
						}
					}()
				} else {
					go func() {
						seed := new([32]byte)
						key, addr := &vanity.Key{}, ""
						for ctx.Err() == nil {
							rand.Read(seed[:])

							key = vanity.KeyFromSeed(seed)
							addr = strings.ToLower(key.Address(network))

							if strings.HasPrefix(addr[initIndex:], pattern) {
								cancel()
								result <- key
								return
							}

							atomic.AddUint64(&ops, 1)
						}
					}()
				}
			}
		}
	}

	seconds := int64(0)
	keyrate := uint64(0)
	lastThree := uint64(0)
	padding := strings.Repeat(" ", 80)
	t := time.NewTicker(time.Second)
	for {
		select {
		case <-t.C:
			seconds++
			last := atomic.LoadUint64(&ops)

			percentStr := "?"
			keyrateStr := "0"
			remainSecStr := "?"

			if diff > 0 {
				percent := float64(last) / float64(diff) * 100
				percentStr = strconv.FormatFloat(percent, 'f', 2, 64)
			}

			if keyrate != 0 {
				keyrateStr = strconv.FormatUint(keyrate, 10)
				if diff > last {
					remain := diff - last
					remainSecStr = (time.Duration(remain/keyrate) * time.Second).String()
				}
			}

			if seconds%3 == 0 {
				keyrate = (last - lastThree) / 3
				lastThree = last
			}

			stats := fmt.Sprintf("Key Rate: %s key/s || Total: %d (%s%%) || Time: %s / %s",
				keyrateStr,
				last,
				percentStr,
				time.Duration(seconds)*time.Second,
				remainSecStr,
			)
			fmt.Printf("%-.80s\r", stats+padding)

		case k := <-result:
			t.Stop()

			if needOnlySpendKey {
				k.HalfToFull()
			}

			fmt.Println()
			fmt.Println("=========================================")
			if wMode == wmStandard {
				words := dict.Encode(k.Seed())
				fmt.Println("Address:")
				fmt.Println(k.Address(network))
				fmt.Println()
				fmt.Println("Mnemonic Seed:")
				fmt.Println(strings.Join(words[:], " "))
				fmt.Println()
				fmt.Println("Private Spend Key:")
				fmt.Printf("%x\n", *k.SpendKey)
				fmt.Println()
				fmt.Println("Private View Key:")
				fmt.Printf("%x\n", *k.ViewKey)
				fmt.Println()
				fmt.Println()
				fmt.Println("HINT: You had better test the mnemonic seeds in Morelo official wallet to check if they are legit. If the seeds work and you want to use the address, write the seeds down on real paper, and never disclose it!")
			}

			exit()
		}
	}
}

func prompt(question string) string {
	for {
		fmt.Print(question + " ")
		stdin.Scan()
		ans := strings.TrimSpace(stdin.Text())
		if ans != "" {
			return ans
		}
		fmt.Println("can't be empty")
	}
}

func promptComfirm(question string) bool {
	return prompt(question) == "y"
}

func promptNumber(question string, min, max int) int {
	for {
		n, err := strconv.Atoi(prompt(question))
		switch {
		case err != nil:
			fmt.Println("invalid number")
		case n < min || n > max:
			fmt.Println("invalid range")
		default:
			return n
		}
	}
}

func prompt256Key(question string) *[32]byte {
	for {
		keyHex := prompt(question)
		if len(keyHex) != 64 {
			fmt.Println("Wrong key size, should be exactly 64 characters")
			continue
		}
		raw, err := hex.DecodeString(keyHex)
		if err != nil {
			fmt.Println(err)
			continue
		}

		ret := new([32]byte)
		copy(ret[:], raw)

		return ret
	}
}

func exit() {
	fmt.Println()
	if runtime.GOOS == "windows" {
		fmt.Println("[Press Enter to exit]")
		stdin.Scan()
	}
	os.Exit(0)
}
