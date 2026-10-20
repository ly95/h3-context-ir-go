package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ly95/contextir"
)

// Header contains facts encoded in a file's header. Reading it does not prove
// that the compressed image or audio/video stream can be decoded, and supplies
// no semantic observation. Unknown formats return an empty Kind and MIME.
// Header recognition never uses a filename or user-authored description.
// Format layouts and tests derive from open-h3-ir analyse.py at fd031e136a
// (Apache-2.0; see third_party/open-h3-ir).
type Header struct {
	Kind   contextir.MediaKind `json:"kind,omitempty"`
	MIME   string              `json:"mime,omitempty"`
	Width  int                 `json:"width,omitempty"`
	Height int                 `json:"height,omitempty"`
}

// ValidateDeclaration checks a visual declaration against identified container
// bytes before a model or decoder is asked to interpret them. Audio declarations
// are outside this header check: an audio-only m4a legitimately uses ISO boxes.
// Unknown headers remain unknown and require the caller's normal decode path.
func ValidateDeclaration(path string, declared contextir.MediaKind) (Header, error) {
	header, err := ReadHeader(path)
	if err != nil {
		return Header{}, err
	}
	if (declared == contextir.MediaImage || declared == contextir.MediaVideo) && header.Kind != "" && declared != header.Kind {
		return header, mediaError(contextir.CodePerception, "kind", fmt.Sprintf("The bytes of %s identify %s media, but it was attached as kind: %s. Attach it with kind: %s instead.", filepath.Base(path), header.Kind, declared, header.Kind), nil)
	}
	return header, nil
}

func ReadHeader(path string) (Header, error) {
	if !filepath.IsAbs(path) {
		return Header{}, mediaError(contextir.CodeInvalid, "path", "A caller-resolved absolute local file path is required.", nil)
	}
	f, err := os.Open(path)
	if err != nil {
		return Header{}, mediaError(contextir.CodePerception, "header", "The media header could not be read.", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return Header{}, mediaError(contextir.CodeInvalid, "header", "A regular media file is required.", err)
	}
	head := make([]byte, 32)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Header{}, err
	}
	head = head[:n]
	out := Header{}
	has := func(offset int, want []byte) bool {
		return offset >= 0 && len(head) >= offset+len(want) && bytes.Equal(head[offset:offset+len(want)], want)
	}
	switch {
	case has(0, []byte("\x89PNG\r\n\x1a\n")):
		out.Kind = contextir.MediaImage
		out.MIME = "image/png"
		if len(head) >= 24 {
			out.Width = int(binary.BigEndian.Uint32(head[16:20]))
			out.Height = int(binary.BigEndian.Uint32(head[20:24]))
		}
	case has(0, []byte("\xff\xd8")):
		out.Kind = contextir.MediaImage
		out.MIME = "image/jpeg"
		out.Width, out.Height, err = jpegDimensions(f)
	case has(0, []byte("GIF87a")) || has(0, []byte("GIF89a")):
		out.Kind = contextir.MediaImage
		out.MIME = "image/gif"
		if len(head) >= 10 {
			out.Width = int(binary.LittleEndian.Uint16(head[6:8]))
			out.Height = int(binary.LittleEndian.Uint16(head[8:10]))
		}
	case has(0, []byte("BM")):
		out.Kind = contextir.MediaImage
		out.MIME = "image/bmp"
		if len(head) >= 26 {
			out.Width = abs32(binary.LittleEndian.Uint32(head[18:22]))
			out.Height = abs32(binary.LittleEndian.Uint32(head[22:26]))
		}
	case has(0, []byte("RIFF")):
		switch {
		case has(8, []byte("WEBP")):
			out.Kind = contextir.MediaImage
			out.MIME = "image/webp"
			switch {
			case has(12, []byte("VP8X")) && len(head) >= 30:
				out.Width = le24(head[24:27]) + 1
				out.Height = le24(head[27:30]) + 1
			case has(12, []byte("VP8 ")) && len(head) >= 30:
				out.Width = int(binary.LittleEndian.Uint16(head[26:28]) & 0x3fff)
				out.Height = int(binary.LittleEndian.Uint16(head[28:30]) & 0x3fff)
			case has(12, []byte("VP8L")) && len(head) >= 25:
				bits := binary.LittleEndian.Uint32(head[21:25])
				out.Width = int(bits&0x3fff) + 1
				out.Height = int((bits>>14)&0x3fff) + 1
			}
		case has(8, []byte("WAVE")):
			out.Kind = contextir.MediaAudio
		case has(8, []byte("AVI ")):
			out.Kind = contextir.MediaVideo
		}
	case has(4, []byte("ftyp")) || has(0, []byte("\x1a\x45\xdf\xa3")):
		out.Kind = contextir.MediaVideo // ISO container ambiguity is retained; Inspect reads actual streams.
	case has(0, []byte("ID3")) || has(0, []byte("\xff\xfb")) || has(0, []byte("\xff\xf3")) || has(0, []byte("\xff\xf2")) || has(0, []byte("fLaC")) || has(0, []byte("OggS")):
		out.Kind = contextir.MediaAudio
	}
	if err != nil {
		return Header{}, err
	}
	return out, nil
}
func abs32(v uint32) int {
	x := int64(int32(v))
	if x < 0 {
		x = -x
	}
	return int(x)
}
func le24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
func jpegDimensions(f *os.File) (int, int, error) {
	if _, e := f.Seek(2, io.SeekStart); e != nil {
		return 0, 0, e
	}
	const maxHeader = 8 << 20
	read := func(n int) ([]byte, error) {
		at, e := f.Seek(0, io.SeekCurrent)
		if e != nil {
			return nil, e
		}
		if at+int64(n) > maxHeader {
			return nil, mediaError(contextir.CodeLimit, "jpeg_header", "JPEG frame header exceeds the explicit 8 MiB scan limit.", nil)
		}
		b := make([]byte, n)
		_, e = io.ReadFull(f, b)
		return b, e
	}
	for {
		b, e := read(1)
		if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
			return 0, 0, nil
		}
		if e != nil {
			return 0, 0, e
		}
		if b[0] != 0xff {
			continue
		}
		for {
			b, e = read(1)
			if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
				return 0, 0, nil
			}
			if e != nil {
				return 0, 0, e
			}
			if b[0] != 0xff {
				break
			}
		}
		m := b[0]
		if m == 0xd8 || m == 0xd9 || (m >= 0xd0 && m <= 0xd7) {
			continue
		}
		size, e := read(2)
		if e != nil {
			if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
				return 0, 0, nil
			}
			return 0, 0, e
		}
		n := int(binary.BigEndian.Uint16(size))
		if n < 2 {
			return 0, 0, nil
		}
		if m >= 0xc0 && m <= 0xcf && m != 0xc4 && m != 0xc8 && m != 0xcc {
			body, e := read(5)
			if e != nil {
				if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
					return 0, 0, nil
				}
				return 0, 0, e
			}
			return int(binary.BigEndian.Uint16(body[3:5])), int(binary.BigEndian.Uint16(body[1:3])), nil
		}
		at, e := f.Seek(int64(n-2), io.SeekCurrent)
		if e != nil {
			return 0, 0, e
		}
		if at > maxHeader {
			return 0, 0, mediaError(contextir.CodeLimit, "jpeg_header", "JPEG frame header exceeds the explicit 8 MiB scan limit.", nil)
		}
	}
}
