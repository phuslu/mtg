package telegram

import "net"
import "github.com/9seconds/mtg/conntypes"

type Telegram interface {
	Dial(conntypes.DC, conntypes.ConnectionProtocol, *net.TCPAddr) (conntypes.StreamReadWriteCloser, error)
	Secret() []byte
}
