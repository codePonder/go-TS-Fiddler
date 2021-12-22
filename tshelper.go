// this package provides support for mpeg2 ts functions. demuxing from file, checking the 4 byte header, pcr checking, PAT/ PMT / SDT and SCTE-35 parsing
package tshelper

import (
	"errors"
	"fmt"
)

// structure of the 4 byte TS header iso 13818
type tsHeaderInfo struct {
	syncByte          uint8  // 8 bits
	transportError    uint8  // 1 bit
	payloadUnitStart  uint8  // 1 bit
	transportPriority uint8  // 1 bit
	pid               uint16 // 13 bits
	scrambling        uint8  // 2 bits
	adaptation        uint8  // 2 bits
	continuity        uint8  // 4 bits
}

func Tshelper() {
	fmt.Println(" == hello from tshelper == ")
}

// function to parse the data sent.  Data must be byte aligned
// start with a 0x47 (ie the start of a TS packet must be first)
// length of blob is number bytes passed in
func parseTSDataBlob(blobData []byte, blobLength uint64) (dataParsed uint64, err error) {

	err = nil
	dataParsed = 0
	blobLength = (blobLength % 188) * 188 // must be multiple of TS packet length
	packetToProcess := uint64(0)
	nextPacket := blobData[packetToProcess:]

	if blobLength == 0 {
		err = errors.New(" TS parsing requires blob to be >= 188 bytes long ")
	} else {
		header := new(tsHeaderInfo)
		for packetToProcess = 0; packetToProcess < blobLength; packetToProcess += 188 {
			nextPacket = blobData[packetToProcess:(packetToProcess + 188)]
			header.syncByte = uint8(nextPacket[0])
			header.transportError = uint8(nextPacket[1])
			header.payloadUnitStart = uint8(nextPacket[1])
			header.transportPriority = uint8(nextPacket[1])
			header.pid = ((uint16(nextPacket[1]) & 0x0f) << 8) + uint16(nextPacket[2])
			header.scrambling = uint8(nextPacket[3]) & 0xc0 >> 6
			header.adaptation = uint8(nextPacket[3]) & 0x30 >> 4
			header.continuity = uint8(nextPacket[3]) & 0x0f
		}
	}

	test := blobData[0]
	fmt.Printf(" first byte 0x%x", test)
	return
}
