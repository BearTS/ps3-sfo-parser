package sfo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

type SFOType byte

const (
	ByteType   SFOType = 0
	StringType SFOType = 2
	IntType    SFOType = 4
)

type PsfHdr struct {
	Psf      [4]byte
	Unknown  [4]byte
	LabelPtr int32
	DataPtr  int32
	NSects   int32
}

type PsfSec struct {
	LabelOff      int16
	Unknown       byte
	DataType      byte
	DatafieldUsed int32
	DatafieldSize int32
	DataOff       int32
}

type SFOPair struct {
	Label  string
	Type   SFOType
	Value  interface{}
	PsfSec PsfSec
}

type SFOParser struct {
	PsfHdr   PsfHdr
	PsfSec   []PsfSec
	Pairs    []SFOPair
	FilePath string
}

func alignment(num, align int) int {
	tmp := num % align
	if tmp != 0 {
		return num + 4 - tmp
	}
	return num
}

func readByteString(r io.Reader) ([]byte, error) {
	var result []byte
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			return nil, err
		}
		if buf[0] == 0 {
			break
		}
		result = append(result, buf[0])
	}
	return result, nil
}

func NewSFOParser(filePath string) (*SFOParser, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.IsDir() {
		return nil, errors.New("not a valid SFO file")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	parser := &SFOParser{FilePath: filePath}
	err = binary.Read(file, binary.LittleEndian, &parser.PsfHdr)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(parser.PsfHdr.Psf[:], []byte{0, 'P', 'S', 'F'}) {
		return nil, errors.New("not a valid SFO file")
	}

	parser.PsfSec = make([]PsfSec, parser.PsfHdr.NSects)
	for i := 0; i < int(parser.PsfHdr.NSects); i++ {
		err = binary.Read(file, binary.LittleEndian, &parser.PsfSec[i])
		if err != nil {
			return nil, err
		}
	}

	parser.Pairs = make([]SFOPair, parser.PsfHdr.NSects)
	for i := 0; i < int(parser.PsfHdr.NSects); i++ {
		file.Seek(int64(parser.PsfSec[i].LabelOff+int16(parser.PsfHdr.LabelPtr)), io.SeekStart)
		tmpbuffer, err := readByteString(file)
		if err != nil {
			return nil, err
		}
		parser.Pairs[i].Label = string(tmpbuffer)
		parser.Pairs[i].PsfSec = parser.PsfSec[i]

		file.Seek(int64(parser.PsfSec[i].DataOff+parser.PsfHdr.DataPtr), io.SeekStart)
		tmpbuffer = make([]byte, parser.PsfSec[i].DatafieldUsed)
		_, err = file.Read(tmpbuffer)
		if err != nil {
			return nil, err
		}
		parser.Pairs[i].Type = SFOType(parser.PsfSec[i].DataType)
		switch parser.PsfSec[i].DataType {
		case 0:
			parser.Pairs[i].Value = tmpbuffer
		case 2:
			parser.Pairs[i].Value = string(tmpbuffer)
		case 4:
			parser.Pairs[i].Value = int32(binary.LittleEndian.Uint32(tmpbuffer))
		}
	}
	return parser, nil
}

func (parser *SFOParser) SaveSFO() error {
	file, err := os.OpenFile(parser.FilePath, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	var buf bytes.Buffer

	parser.PsfHdr.LabelPtr = int32(binary.Size(parser.PsfHdr) + len(parser.PsfSec)*binary.Size(PsfSec{}))

	// Write label field
	buf.Reset()
	for i := 0; i < int(parser.PsfHdr.NSects); i++ {
		parser.PsfSec[i].LabelOff = int16(buf.Len())
		buf.WriteString(parser.Pairs[i].Label)
		buf.WriteByte(0)
	}
	parser.PsfHdr.DataPtr = int32(alignment(buf.Len(), 4))

	for buf.Len() < int(parser.PsfHdr.DataPtr) {
		buf.WriteByte(0)
	}

	// Write data set
	for i := 0; i < int(parser.PsfHdr.NSects); i++ {
		parser.PsfSec[i].DataOff = int32(buf.Len())
		switch parser.Pairs[i].Type {
		case 0:
			buf.Write(parser.Pairs[i].Value.([]byte))
		case 2:
			buf.WriteString(parser.Pairs[i].Value.(string))
		case 4:
			binary.Write(&buf, binary.LittleEndian, parser.Pairs[i].Value.(int32))
		}

		parser.PsfSec[i].DatafieldUsed = int32(buf.Len()) - parser.PsfSec[i].DataOff
		parser.PsfSec[i].DatafieldSize = int32(alignment(int(parser.PsfSec[i].DatafieldUsed), 4))
		for buf.Len() < int(parser.PsfSec[i].DataOff+parser.PsfSec[i].DatafieldSize) {
			buf.WriteByte(0)
		}
	}

	// Write PsfSec
	file.Seek(int64(binary.Size(parser.PsfHdr)), io.SeekStart)
	for _, sec := range parser.PsfSec {
		binary.Write(file, binary.LittleEndian, sec)
	}
	parser.PsfHdr.LabelPtr = int32(binary.Size(parser.PsfHdr) + len(parser.PsfSec)*binary.Size(PsfSec{}))

	// Write PsfHdr
	file.Seek(0, io.SeekStart)
	binary.Write(file, binary.LittleEndian, parser.PsfHdr)

	return nil
}

func (parser *SFOParser) GetValue(key string) (interface{}, error) {
	for _, pair := range parser.Pairs {
		if pair.Label == key {
			return pair.Value, nil
		}
	}
	return nil, errors.New("key not found")
}

func (parser *SFOParser) GetLength() int {
	return int(parser.PsfHdr.NSects)
}

func (parser *SFOParser) GetValueByIndex(index int) (interface{}, error) {
	if index < 0 || index >= len(parser.Pairs) {
		return nil, errors.New("index out of range")
	}
	return parser.Pairs[index].Value, nil
}

func (parser *SFOParser) GetKeyByIndex(index int) (string, error) {
	if index < 0 || index >= len(parser.Pairs) {
		return "", errors.New("index out of range")
	}
	return parser.Pairs[index].Label, nil
}

func (parser *SFOParser) GetTypeByIndex(index int) (byte, error) {
	if index < 0 || index >= len(parser.Pairs) {
		return 0, errors.New("index out of range")
	}
	return byte(parser.Pairs[index].Type), nil
}

func (parser *SFOParser) SetValueByIndex(index int, value string) error {
	if index < 0 || index >= len(parser.Pairs) {
		return errors.New("index out of range")
	}
	switch parser.Pairs[index].Type {
	case ByteType:
		parser.Pairs[index].Value = []byte(value)
	case StringType:
		if value[len(value)-1] != 0 {
			value += "\x00"
		}
		parser.Pairs[index].Value = value
	case IntType:
		var intValue int32
		_, err := fmt.Sscanf(value, "%d", &intValue)
		if err != nil {
			return err
		}
		parser.Pairs[index].Value = intValue
	default:
		return errors.New("unknown type")
	}
	return nil
}

func (parser *SFOParser) SetLabelByIndex(index int, value string) error {
	if index < 0 || index >= len(parser.Pairs) {
		return errors.New("index out of range")
	}
	parser.Pairs[index].Label = value
	return nil
}
