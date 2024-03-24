package vanity

func Uint64ToBytes(num uint64) (result []byte) {
	for ; num >= 0x80; num >>= 7 {
		result = append(result, byte((num&0x7f)|0x80))
	}
	result = append(result, byte(num))
	return
}

type Network []byte

var (
	MoreloMainNetwork  = Uint64ToBytes(0x1a29e1)
)
