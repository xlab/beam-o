package proto

// AUTO GENERATED - DO NOT EDIT

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	C "github.com/glycerine/go-capnproto"
)

type FileHeader C.Struct

func NewFileHeader(s *C.Segment) FileHeader      { return FileHeader(s.NewStruct(24, 1)) }
func NewRootFileHeader(s *C.Segment) FileHeader  { return FileHeader(s.NewRootStruct(24, 1)) }
func AutoNewFileHeader(s *C.Segment) FileHeader  { return FileHeader(s.NewStructAR(24, 1)) }
func ReadRootFileHeader(s *C.Segment) FileHeader { return FileHeader(s.Root(0).ToStruct()) }
func (s FileHeader) Name() string                { return C.Struct(s).GetObject(0).ToText() }
func (s FileHeader) NameBytes() []byte           { return C.Struct(s).GetObject(0).ToDataTrimLastByte() }
func (s FileHeader) SetName(v string)            { C.Struct(s).SetObject(0, s.Segment.NewText(v)) }
func (s FileHeader) IsDir() bool                 { return C.Struct(s).Get1(0) }
func (s FileHeader) SetIsDir(v bool)             { C.Struct(s).Set1(0, v) }
func (s FileHeader) Mode() uint32                { return C.Struct(s).Get32(4) }
func (s FileHeader) SetMode(v uint32)            { C.Struct(s).Set32(4, v) }
func (s FileHeader) ModTime() int64              { return int64(C.Struct(s).Get64(8)) }
func (s FileHeader) SetModTime(v int64)          { C.Struct(s).Set64(8, uint64(v)) }
func (s FileHeader) Size() int64                 { return int64(C.Struct(s).Get64(16)) }
func (s FileHeader) SetSize(v int64)             { C.Struct(s).Set64(16, uint64(v)) }
func (s FileHeader) WriteJSON(w io.Writer) error {
	b := bufio.NewWriter(w)
	var err error
	var buf []byte
	_ = buf
	err = b.WriteByte('{')
	if err != nil {
		return err
	}
	_, err = b.WriteString("\"name\":")
	if err != nil {
		return err
	}
	{
		s := s.Name()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte(',')
	if err != nil {
		return err
	}
	_, err = b.WriteString("\"isDir\":")
	if err != nil {
		return err
	}
	{
		s := s.IsDir()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte(',')
	if err != nil {
		return err
	}
	_, err = b.WriteString("\"mode\":")
	if err != nil {
		return err
	}
	{
		s := s.Mode()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte(',')
	if err != nil {
		return err
	}
	_, err = b.WriteString("\"modTime\":")
	if err != nil {
		return err
	}
	{
		s := s.ModTime()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte(',')
	if err != nil {
		return err
	}
	_, err = b.WriteString("\"size\":")
	if err != nil {
		return err
	}
	{
		s := s.Size()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte('}')
	if err != nil {
		return err
	}
	err = b.Flush()
	return err
}
func (s FileHeader) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	err := s.WriteJSON(&b)
	return b.Bytes(), err
}
func (s FileHeader) WriteCapLit(w io.Writer) error {
	b := bufio.NewWriter(w)
	var err error
	var buf []byte
	_ = buf
	err = b.WriteByte('(')
	if err != nil {
		return err
	}
	_, err = b.WriteString("name = ")
	if err != nil {
		return err
	}
	{
		s := s.Name()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	_, err = b.WriteString(", ")
	if err != nil {
		return err
	}
	_, err = b.WriteString("isDir = ")
	if err != nil {
		return err
	}
	{
		s := s.IsDir()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	_, err = b.WriteString(", ")
	if err != nil {
		return err
	}
	_, err = b.WriteString("mode = ")
	if err != nil {
		return err
	}
	{
		s := s.Mode()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	_, err = b.WriteString(", ")
	if err != nil {
		return err
	}
	_, err = b.WriteString("modTime = ")
	if err != nil {
		return err
	}
	{
		s := s.ModTime()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	_, err = b.WriteString(", ")
	if err != nil {
		return err
	}
	_, err = b.WriteString("size = ")
	if err != nil {
		return err
	}
	{
		s := s.Size()
		buf, err = json.Marshal(s)
		if err != nil {
			return err
		}
		_, err = b.Write(buf)
		if err != nil {
			return err
		}
	}
	err = b.WriteByte(')')
	if err != nil {
		return err
	}
	err = b.Flush()
	return err
}
func (s FileHeader) MarshalCapLit() ([]byte, error) {
	b := bytes.Buffer{}
	err := s.WriteCapLit(&b)
	return b.Bytes(), err
}

type FileHeader_List C.PointerList

func NewFileHeaderList(s *C.Segment, sz int) FileHeader_List {
	return FileHeader_List(s.NewCompositeList(24, 1, sz))
}
func (s FileHeader_List) Len() int            { return C.PointerList(s).Len() }
func (s FileHeader_List) At(i int) FileHeader { return FileHeader(C.PointerList(s).At(i).ToStruct()) }
func (s FileHeader_List) ToArray() []FileHeader {
	n := s.Len()
	a := make([]FileHeader, n)
	for i := 0; i < n; i++ {
		a[i] = s.At(i)
	}
	return a
}
func (s FileHeader_List) Set(i int, item FileHeader) { C.PointerList(s).Set(i, C.Object(item)) }
