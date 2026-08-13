package jxl

import (
	"encoding/binary"
	"sort"
)

type container struct {
	codestream []byte
	exif       []byte
	xmp        []byte
	jumb       []byte
	level      int
	compressed []string
}

type jxlpChunk struct {
	index uint32
	data  []byte
}

func parseContainer(data []byte) (*container, error) {
	return readContainer(data, false)
}

// readContainer parses the boxes. With partial set, a final box that runs past
// the buffer is clamped to what is there and the walk stops, which is what
// DecodeConfig needs to reach the image header without the whole file.
func readContainer(data []byte, partial bool) (*container, error) {
	c := &container{level: 5}

	var (
		chunks   []jxlpChunk
		seenFtyp bool
		version  uint32
		seenJxlc bool
		lastJxlp = -1
	)

	for pos := 0; pos < len(data); {
		if len(data)-pos < 8 {
			return nil, ErrInvalid
		}

		size := uint64(binary.BigEndian.Uint32(data[pos:]))
		boxType := string(data[pos+4 : pos+8])
		header := 8

		if size == 1 {
			if len(data)-pos < 16 {
				return nil, ErrInvalid
			}

			size = binary.BigEndian.Uint64(data[pos+8:])
			header = 16
		}

		if size == 0 {
			size = uint64(len(data) - pos)
		}

		if size < uint64(header) {
			return nil, ErrInvalid
		}

		if size > uint64(len(data)-pos) {
			if !partial {
				return nil, ErrInvalid
			}

			size = uint64(len(data) - pos)
		}

		body := data[pos+header : pos+int(size)]
		pos += int(size)

		if !seenFtyp && boxType != "ftyp" {
			return nil, ErrInvalid
		}

		switch boxType {
		case "ftyp":
			if seenFtyp || len(body) < 8 || string(body[:4]) != "jxl " {
				return nil, ErrInvalid
			}

			version = binary.BigEndian.Uint32(body[4:])
			if version > 1 {
				return nil, ErrInvalid
			}

			seenFtyp = true

		case "jxlc":
			if seenJxlc || len(chunks) > 0 {
				return nil, ErrInvalid
			}

			seenJxlc = true
			c.codestream = body

		case "jxlp":
			if seenJxlc || len(body) < 4 {
				return nil, ErrInvalid
			}

			index := binary.BigEndian.Uint32(body)
			last := index&0x80000000 != 0
			index &= 0x7fffffff

			if index != uint32(len(chunks)) && version != 1 {
				return nil, ErrInvalid
			}
			if lastJxlp >= 0 && (last || index >= uint32(lastJxlp)) {
				return nil, ErrInvalid
			}

			for _, ch := range chunks {
				if ch.index == index {
					return nil, ErrInvalid
				}
			}

			if last {
				lastJxlp = int(index)
			}

			chunks = append(chunks, jxlpChunk{index, body[4:]})

		case "jxll":
			if len(body) < 1 {
				return nil, ErrInvalid
			}

			c.level = int(body[0])

		case "Exif":
			c.exif = body
		case "xml ":
			c.xmp = body
		case "jumb":
			c.jumb = body

		case "brob":
			if len(body) < 4 {
				return nil, ErrInvalid
			}

			c.compressed = append(c.compressed, string(body[:4]))

		}
	}

	if !seenFtyp {
		return nil, ErrInvalid
	}

	if len(chunks) > 0 {
		if lastJxlp != len(chunks)-1 && !partial {
			return nil, ErrInvalid
		}

		sort.Slice(chunks, func(i, j int) bool { return chunks[i].index < chunks[j].index })

		total := 0
		for i, ch := range chunks {
			if ch.index != uint32(i) {
				if partial {
					chunks = chunks[:i]

					break
				}

				return nil, ErrInvalid
			}

			total += len(ch.data)
		}

		c.codestream = make([]byte, 0, total)
		for _, ch := range chunks {
			c.codestream = append(c.codestream, ch.data...)
		}
	}

	if c.codestream == nil {
		return nil, ErrInvalid
	}

	return c, nil
}
