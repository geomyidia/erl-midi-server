package erl

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	erlang "github.com/okeuday/erlang_go/v2/erlang"
	log "github.com/sirupsen/logrus"

	"github.com/geomyidia/erl-midi-server/pkg/types"
)

// Constants
const (
	DELIMITER       = '\n'
	DRCTVARITY      = 2
	DRCTVKEYINDEX   = 0
	DRCTVVALUEINDEX = 1
)

func Continue() types.Result {
	return types.Result("continue")
}

type Opts struct {
	IsHexEncoded bool
}

func DefaultOpts() *Opts {
	return &Opts{
		IsHexEncoded: false,
	}
}

type Packet struct {
	bytes []byte
	len   int
	last  int
	opts  *Opts
}

func ReadStdIOPacket(opts *Opts) (*Packet, error) {
	reader := bufio.NewReader(os.Stdin)
	bytes, _ := reader.ReadBytes(DELIMITER)
	byteLen := len(bytes)
	if byteLen == 0 {
		return nil, errors.New("read zero bytes")
	}
	log.Tracef("original packet: %#v", bytes)
	log.Tracef("original packet length: %d", byteLen)
	packet := &Packet{
		bytes: bytes,
		len:   byteLen,
		last:  byteLen - 1,
		opts:  opts,
	}
	return packet, nil
}

func (p *Packet) getTrimmed() []byte {
	log.Debug("getting trimmed ...")
	return p.bytes[:p.last]
}

func (p *Packet) Bytes() ([]byte, error) {
	log.Debug("getting bytes ...")
	log.Debugf("IsHexEncoded: %v", p.opts.IsHexEncoded)
	if p.opts.IsHexEncoded {
		return p.getUnwrapped()
	}
	return p.getTrimmed(), nil
}

// setUnwrapped is a utility method for a hack needed in order to
// successully handle messages from the Erlang exec library.
//
// What was happening when exec messages were being processed
// by ProcessPortMessage was that a single byte was being dropped
// from the middle (in the case of the #(command ping) message,
// it was byte 0x04 of the Term protocol encoded bytes). The
// bytes at the sending end were present and correct, just not
// at the receiving end.
//
// So, in order to get around this, the sending end hex-encoded
// the Term protocol bytes and send that as a bitstring; the
// function below hex-decodes this, and allows the function
// ProcessExecMessage to handle binary encoded Term data with
// none of its bytes missing.
func (p *Packet) getUnwrapped() ([]byte, error) {
	log.Debug("getting unwrapped ... ")
	if p.opts.IsHexEncoded {
		hexStr := string(p.getTrimmed()[:])
		log.Tracef("got hex string: %s", hexStr)
		bytes, err := hex.DecodeString(hexStr)
		log.Tracef("got decoded string: %v", bytes)
		if err != nil {
			return nil, fmt.Errorf("problem unwrapping packet: %s", err.Error())
		}
		log.Tracef("set trim bytes: %v", bytes)
		return bytes, nil
	}
	return nil, nil
}

func (p *Packet) Term() (interface{}, error) {
	log.Debug("getting term ...")
	bytes, err := p.Bytes()
	if err != nil {
		return nil, fmt.Errorf("problem getting bytes %#v: %s",
			bytes, err.Error())
	}
	log.Tracef("got bytes: %v", bytes)
	term, err := erlang.BinaryToTerm(bytes)
	if err != nil {
		return nil, fmt.Errorf("problem creating Erlang term from %#v: %s",
			bytes, err.Error())
	}
	return term, nil
}

type Message struct {
	tuple     erlang.OtpErlangTuple
	directive erlang.OtpErlangAtom
	payload   interface{}
}

func NewMessage(t interface{}) (*Message, error) {
	tuple, ok := t.(erlang.OtpErlangTuple)
	if !ok {
		return nil, errors.New("unexpected message format")
	}
	if len(tuple) != DRCTVARITY {
		return nil, fmt.Errorf("tuple of wrong size; expected 2, got %d", len(tuple))
	}
	directive, ok := tuple[DRCTVKEYINDEX].(erlang.OtpErlangAtom)
	if !ok {
		return nil, errors.New("unexpected type for directive")
	}
	msg := &Message{tuple: tuple}
	msg.directive = directive
	msg.payload = tuple[DRCTVVALUEINDEX]
	return msg, nil
}

func (m *Message) Directive() erlang.OtpErlangAtom {
	return m.directive
}

func (m *Message) Payload() interface{} {
	return m.payload
}

func (m *Message) IsCommand() bool {
	return m.directive == erlang.OtpErlangAtom("command")
}

func (m *Message) IsMIDI() bool {
	return m.directive == erlang.OtpErlangAtom("midi")
}

func (m *Message) Command() (erlang.OtpErlangAtom, error) {
	if !m.IsCommand() {
		return erlang.OtpErlangAtom("error"),
			errors.New("directive is not a command")
	}
	command, ok := m.Payload().(erlang.OtpErlangAtom)
	if !ok {
		return erlang.OtpErlangAtom("error"),
			errors.New("could not extract command atom")
	}
	return command, nil
}

type MessageProcessor struct {
	packet *Packet
	term   interface{}
	msg    *Message
}

func NewMessageProcessor(opts *Opts) (*MessageProcessor, error) {
	packet, err := ReadStdIOPacket(opts)
	if err != nil {
		return &MessageProcessor{}, err
	}
	t, err := packet.Term()
	if err != nil {
		return &MessageProcessor{}, err
	}
	log.Debugf("got Erlang Port term")
	log.Tracef("%#v", t)
	msg, err := NewMessage(t)
	if err != nil {
		resp := NewResponse(types.Result(""), types.Err(err.Error()))
		resp.Send()
		return &MessageProcessor{}, err
	}
	return &MessageProcessor{
		packet: packet,
		term:   t,
		msg:    msg,
	}, nil
}

func (mp *MessageProcessor) Continue() types.Result {
	return types.Result("continue")
}

func (mp *MessageProcessor) Process() types.Result {
	if mp.msg.IsCommand() {
		command, err := mp.msg.Command()
		if err != nil {
			log.Error(err)
			return mp.Continue()
		}
		return types.Result(command)
	} else if mp.msg.IsMIDI() {
		// process MIDI message
		return mp.Continue()
	} else {
		log.Error("unexected message type")
		return mp.Continue()
	}
}