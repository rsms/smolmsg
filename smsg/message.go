// SPDX-License-Identifier: Apache-2.0
package main

import (
  "bufio"
  "bytes"
  "io"
  "os"
  "path/filepath"
  "strconv"
  "strings"
  "time"

  "golang.org/x/text/unicode/norm"
)

const (
  FIELD_SUBJECT int = iota
  FIELD_FROM
  FIELD_TO
  FIELD_TIME
  FIELD_BODY
  FIELD_FILE
)

var fieldtab map[string]int // maps field name to FIELD_ constant

const MAX_BODY_SIZE = 8 * 1024 * 1024 // 8 MiB

// idEpochBase offsets the timestamp to provide a wider range.
// Effective range (0x0–0xFFFFFFFF): 2020-09-13 12:26:40 – 2156-10-20 18:54:55 (UTC)
const idEpochBase int64 = 1600000000

const base62Characters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type Author struct {
  address string
  name    string
}

func (a Author) String() string {
  if a.name == "" {
    return a.address
  }
  return strconv.Quote(a.name) + " " + a.address
}

func (a Author) ShortString() string {
  if a.name == "" {
    return a.address
  }
  return a.name
}

func (a *Author) Parse(line []byte) (err error) {
  line = bytes.TrimSpace(line)
  if len(line) == 0 {
    return errorf("missing address")
  }
  if p := bytes.IndexByte(line, ' '); p != -1 {
    a.name = string(bytes.TrimSpace(line[p:]))
    line = line[:p]
  }
  a.address, err = normalizeAndValidateAddress(string(line))
  return err
}

type Attachment struct {
  name      string
  dataStart int
  dataLen   int
}

type Message struct {
  id       [24]byte // time + SHA256
  time     time.Time
  subject  string
  from, to Author
  body     []byte
  files    []Attachment
}

func (m *Message) Id() []byte {
  return m.id[:]
}

func (m *Message) String() string {
  return m.time.Format("20060102-150405<") + m.from.address + ">"
}

func (m *Message) SetTimeFromFilename(file string) error {
  file = filepath.Base(file)
  p := strings.IndexByte(file, '.')
  if p == -1 {
    return errorf("invalid message filename %q", file)
  }
  file = file[:p]
  t, err := time.Parse("20060102-150405", file)
  if err != nil {
    return errorf("invalid message filename %q", file)
  }
  m.time = t
  return nil
}

func (m *Message) SetTimeFromId() {
  m.time = m.IdTime()
}

// IdTimestamp returns the timestamp portion of the id
func (m *Message) IdTimestamp() uint32 {
  return uint32(m.id[0])<<24 | uint32(m.id[1])<<16 | uint32(m.id[2])<<8 | uint32(m.id[3])
}

// IdTime returns the time portion of the id
func (m *Message) IdTime() time.Time {
  return time.Unix(int64(m.IdTimestamp())+idEpochBase, 0)
}

func (m *Message) IdString() string {
  var buf [50]byte
  return string(m.EncodeId(buf[:]))
}

// EncodeId writes m.id to dst which must be at least 50 bytes.
// Returns the start offset (this function starts writing at the end of dst.)
func (m *Message) EncodeId(dst []byte) []byte {
  // see https://github.com/rsms/go-uuid/blob/master/uuid.go#L250 for decoder
  const srcBase = 0x100000000
  const dstBase = 62

  parts := [6]uint32{
    uint32(m.id[0])<<24 | uint32(m.id[1])<<16 | uint32(m.id[2])<<8 | uint32(m.id[3]),
    uint32(m.id[4])<<24 | uint32(m.id[5])<<16 | uint32(m.id[6])<<8 | uint32(m.id[7]),
    uint32(m.id[8])<<24 | uint32(m.id[9])<<16 | uint32(m.id[10])<<8 | uint32(m.id[11]),
    uint32(m.id[12])<<24 | uint32(m.id[13])<<16 | uint32(m.id[14])<<8 | uint32(m.id[15]),
    uint32(m.id[16])<<24 | uint32(m.id[17])<<16 | uint32(m.id[18])<<8 | uint32(m.id[19]),
    uint32(m.id[20])<<24 | uint32(m.id[21])<<16 | uint32(m.id[22])<<8 | uint32(m.id[23]),
  }

  n := len(dst)
  bp := parts[:]
  bq := [6]uint32{}

  for len(bp) != 0 {
    quotient := bq[:0]
    remainder := uint64(0)

    for _, c := range bp {
      value := uint64(c) + uint64(remainder)*srcBase
      digit := value / dstBase
      remainder = value % dstBase

      if len(quotient) != 0 || digit != 0 {
        quotient = append(quotient, uint32(digit))
      }
    }

    // Writes at the end of the destination buffer because we computed the
    // lowest bits first.
    n--
    dst[n] = base62Characters[remainder]
    bp = quotient
  }
  n--
  dst[n] = '0'
  return dst[n-1:]
}

func (m *Message) UpdateIdFromTime() error {
  ut := m.time.Unix()
  if ut < idEpochBase {
    return errorf("invalid timestamp; %s is in the past", m.time.Format(time.RFC3339))
  }

  // second part
  sec := uint32(ut - idEpochBase)
  m.id[0] = byte(sec >> 24)
  m.id[1] = byte(sec >> 16)
  m.id[2] = byte(sec >> 8)
  m.id[3] = byte(sec)

  // // millisecond part
  // ms := uint16(uint64(m.time.Nanosecond()) / uint64(time.Millisecond))
  // m.id[4] = byte(ms >> 8)
  // m.id[5] = byte(ms)

  return nil
}

func (m *Message) ParseReader(r io.Reader, srcsize int, srcname string) error {
  bufsize := srcsize + 1 // one byte for final read at EOF
  if bufsize > 4096 {
    bufsize = 4096
  }
  bufsize = int(1) << ilog2(uint64(bufsize)) // round to nearest (floor) pow2

  var lineno, fileno int
  cr := MakeSHA256HashingCountingReader(r)
  br := bufio.NewReaderSize(&cr, bufsize)
  for {
    lineno++
    line, tooLargeForBuffer, err := br.ReadLine()
    if err != nil {
      if err == io.EOF {
        break
      }
      return err
    }
    if tooLargeForBuffer {
      return errorf("%s:%d: field too long", srcname, lineno)
    }
    //dlog("%4d> %q", lineno, line)

    // parse field
    p := bytes.IndexByte(line, ' ')
    if p == 0 {
      return errorf("%s:%d: invalid leading space", srcname, lineno)
    }
    if p == -1 {
      if len(line) == 0 { // skip empty line
        continue
      }
      p = len(line)
    }
    key := line[:p]
    field, ok := fieldtab[string(key)]
    if !ok {
      if strings.HasPrefix(string(key), "x-") {
        // ignore "x-*" fields
        continue
      }
      return errorf("%s:%d: unknown field %q", srcname, lineno, key)
    }

    // parse field value
    switch field {

    case FIELD_SUBJECT: // "subject" <text>
      m.subject = string(bytes.TrimSpace(line[p:]))

    case FIELD_FROM: // "from" <address> [<text>]
      if err := m.from.Parse(line[p:]); err != nil {
        return errorf("%s:%d: %s (%q)", srcname, lineno, err, line)
      }

    case FIELD_TO: // "to" <address> [<text>]
      if err := m.to.Parse(line[p:]); err != nil {
        return errorf("%s:%d: %s (%q)", srcname, lineno, err, line)
      }

    case FIELD_TIME: // "time" <datetime> [<timezoneoffset>]
      // e.g. "2006-01-02 15:04:05 -07:00"
      // e.g. "2006-01-02 15:04:05"
      s := string(bytes.TrimSpace(line[p:]))
      format := "2006-01-02 15:04:05 -0700"
      t, err := time.Parse(format, s)
      if err != nil {
        t, err = time.Parse("2006-01-02 15:04:05", s)
        if err != nil {
          return errorf("invalid time format %q (expected %q) %v", s, format, err)
        }
      }
      m.time = t

    case FIELD_BODY: // "body" <bytesize>
      size, err := strconv.ParseUint(string(bytes.TrimSpace(line[p:])), 10, 64)
      if err != nil {
        return errorf("%s:%d: invalid integer size %q", srcname, lineno, line[p:])
      }
      if size > MAX_BODY_SIZE {
        return errorf("%s:%d: body too large (%d)", srcname, lineno, size)
      }
      m.body = make([]byte, size)
      n, err := io.ReadFull(br, m.body)
      if err != nil {
        m.body = m.body[:0]
        return err
      }
      if n != len(m.body) {
        m.body = m.body[:0]
        return errorf("%s:%d: invalid body size %d (beyond end of message file)",
          srcname, lineno, size)
      }

    case FIELD_FILE: // "file" <bytesize> [<text>]
      fileno++
      line = bytes.TrimSpace(line[p:])
      var file Attachment
      if p := bytes.IndexByte(line, ' '); p != -1 {
        file.name = string(bytes.TrimSpace(line[p:]))
        line = line[:p]
      }
      size64, err := strconv.ParseUint(string(line), 10, strconv.IntSize)
      if err != nil {
        return errorf("%s:%d: invalid integer size %q", srcname, lineno, line[p:])
      }
      size := int(size64)
      file.dataStart = cr.nread - br.Buffered()
      discarded, err := br.Discard(int(size))
      if discarded < int(size) {
        return errorf("%s:%d: file %d %q: invalid size %d (beyond end of message file)",
          srcname, lineno, fileno, file.name, size)
      }
      if err != nil {
        return err
      }
      file.dataLen = size
      m.files = append(m.files, file)

    }
  }

  //err := binary.Write(cr.hash, binary.BigEndian, m.time.Unix())
  // cr.hash.Sum(m.id[4:4])
  var buf [32]byte
  cr.hash.Sum(buf[:0])
  copy(m.id[4:], buf[:20])

  return m.UpdateIdFromTime()
}

func (m *Message) ParseFile(srcfile string) error {
  if err := m.SetTimeFromFilename(srcfile); err != nil {
    return err
  }

  f, err := os.Open(srcfile)
  if err != nil {
    return err
  }
  defer f.Close()

  var size int
  if info, err := f.Stat(); err == nil {
    size64 := info.Size()
    if int64(int(size64)) == size64 {
      size = int(size64)
    }
  }
  return m.ParseReader(f, size, srcfile)
}

func normalizeAndValidateAddress(address string) (string, error) {
  // make sure "café" and "café" use the same UTF-8 sequences
  address = norm.NFC.String(address)
  address = strings.ToLower(address)

  // TODO proper validation
  if p := strings.IndexByte(address, '@'); p == -1 {
    return "", errorf("invalid address")
  }
  return address, nil
}

func init() {
  fieldtab = map[string]int{
    "subject": FIELD_SUBJECT,
    "from":    FIELD_FROM,
    "to":      FIELD_TO,
    "time":    FIELD_TIME,
    "body":    FIELD_BODY,
    "file":    FIELD_FILE,
  }
}
