package x11

import (
	"encoding/binary"
	"io"
)

type Family uint16

const (
	FamilyIpv4              Family = 0
	FamilyDECnet            Family = 1
	FamilyChaos             Family = 2
	FamilyServerInterpreted Family = 5
	FamilyIpv6              Family = 6
	FamilyLocalhost         Family = 252
	FamilyKerberos          Family = 253
	FamilyNetName           Family = 254
	FamilyLocal             Family = 256
	FamilyWild              Family = 65535
)

type Entry struct {
	Family              Family
	Host                []byte
	Display             []byte
	AuthorisationMethod []byte
	AuthorisationData   []byte
}

func ProcessFile(r io.Reader) ([]Entry, error) {
	var entries []Entry

	family := make([]byte, 2)
	for {
		var entry Entry
		_, err := r.Read(family)
		if err != nil {
			if len(entries) == 0 {
				return nil, err
			}
			return entries, nil
		}

		entry.Family = Family(binary.BigEndian.Uint16(family))
		err = ReadNext(r, &entry.Host)
		if err != nil {
			return nil, err
		}

		err = ReadNext(r, &entry.Display)
		if err != nil {
			return nil, err
		}

		err = ReadNext(r, &entry.AuthorisationMethod)
		if err != nil {
			return nil, err
		}

		err = ReadNext(r, &entry.AuthorisationData)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}
}

func EncodeEntries(entries []Entry) []byte {
	var b []byte
	for _, e := range entries {
		b = binary.BigEndian.AppendUint16(b, uint16(e.Family))
		b = binary.BigEndian.AppendUint16(b, uint16(len(e.Host)))
		b = append(b, e.Host...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(e.Display)))
		b = append(b, e.Display...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(e.AuthorisationMethod)))
		b = append(b, e.AuthorisationMethod...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(e.AuthorisationData)))
		b = append(b, e.AuthorisationData...)
	}
	return b
}

func ReadNext(r io.Reader, dest *[]byte) error {
	l := make([]byte, 2)
	_, err := r.Read(l)
	if err != nil {
		return err
	}

	*dest = make([]byte, binary.BigEndian.Uint16(l))
	_, err = r.Read(*dest)
	if err != nil {
		return err
	}
	return nil
}
