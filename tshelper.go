// this package provides support for mpeg2 ts functions. demuxing from file, checking the 4 byte header, pcr checking, PAT/ PMT / SDT and SCTE-35 parsing
package tshelper

import (
	"errors"
	"fmt"
)

// the data structure that is the TS-Demultiplxer
type tsdmx struct {
	pidStats map[uint16]pidInfo
	globalStats globalInfo
	tables tableParser
}

// information on what we have seen on individual PIDs
type pidInfo struct {
	lastContCount uint8
	contCountErrors uint64
	packetCount uint64
}


// information on what we have seen generic to the wholestream
type globalInfo struct {
	totalPackets uint64
}


// structure of the 4 byte TS header iso 13818
type tsHeaderInfo struct {
	syncByte          uint8  // 8 bits
	transportError    uint8  // 1 bit
	payloadUnitStart  uint8  // 1 bit
	transportPriority uint8  // 1 bit
	pid               uint16 // 13 bits
	scrambling        uint8  // 2 bits
	adaptation        uint8  // 2 bits
	contCount   	  uint8  // 4 bits
}

// structure of possible adaptation fields in a TS packet
type tsAdaptInfo struct {
	discontinuityFlag uint8 
	raiflag uint8
	espiflag uint8          
	pcrFlag  uint8                 
	opcrFlag uint8
	splicePointFlag uint8
}


// Allow external code to create a tsdmux block (aka == The Constructor == )
func Newtsdmx ( ) tsdmx {
	newStruct := tsdmx{}
	newStruct.pidStats = make(map[uint16]pidInfo)
	newStruct.tables = newTableParser()
	return newStruct
}


// extract bitfield meanings from the 4 Byte TS header (ISO13818) 
func parseTSHeader (nextPacket []byte) (header *tsHeaderInfo) {
	header = new(tsHeaderInfo)
	header.syncByte = uint8(nextPacket[0])
	header.transportError = (uint8(nextPacket[1]) & 0x80) >> 7
	header.payloadUnitStart = (uint8(nextPacket[1]) & 0x40) >> 6
	header.transportPriority = (uint8(nextPacket[1]) & 0x20) >> 5
	header.pid = ((uint16(nextPacket[1]) & 0x1f) << 8) + uint16(nextPacket[2])
	header.scrambling = uint8(nextPacket[3]) & 0xc0 >> 6
	header.adaptation = uint8(nextPacket[3]) & 0x30 >> 4
	header.contCount  = uint8(nextPacket[3]) & 0x0f
	return 
}

// extract bitfield meaning from the adaptation field flags
func parseTSAdaptFields (adaptationByte uint8, tsAdaptFields *tsAdaptInfo) {
	tsAdaptFields = new(tsAdaptInfo)
	tsAdaptFields.discontinuityFlag = adaptationByte & 0x80;
	tsAdaptFields.raiflag           = adaptationByte & 0x40;          
	tsAdaptFields.espiflag          = adaptationByte & 0x20;          
	tsAdaptFields.pcrFlag           = adaptationByte & 0x10;                    
	tsAdaptFields.opcrFlag          = adaptationByte & 0x08;
	tsAdaptFields.splicePointFlag   = adaptationByte & 0x04;
}

// extract the pcr from the adaptation fields in the packet
// pass in pointer to start of adaptation field, return PCR as uint64
func extractPCR (adaptationData []byte) (pcr27Mhz uint64) {
	top32Bits := (uint64(adaptationData[0]) << 24) +
				(uint64(adaptationData[1])	   << 16) +
				(uint64(adaptationData[2])	   <<  8) +
				(uint64(adaptationData[3]) 	   <<  0)
	lsb         := (uint64(adaptationData[4]) >> 7) & 1
	top33Bits   := (top32Bits * 2) + lsb;
	bottom9Bits := ((uint64(adaptationData[4]) & 1) << 8) + uint64(adaptationData[5]);
	pcr27Mhz    = (top33Bits * 300) + bottom9Bits
	return
}


// Parse the data sent.  Data must be byte aligned
// start with a 0x47 (ie the start of a TS packet must be first)
// length of blob is number bytes passed in
func (metaInfo tsdmx) ParseTSDataBlob(blobData []byte, blobLength uint64) (dataParsed uint64, err error) {
	err = nil
	dataParsed = 0
	blobLength = (blobLength / 188) * 188 // must be multiple of TS packet length

	if blobLength == 0 {
		err = errors.New(" TS parsing requires blob to be >= 188 bytes long ")
	} else {
		for packetToProcess := uint64(0); packetToProcess < blobLength; packetToProcess += 188 {
			nextPacket := blobData[packetToProcess:(packetToProcess + 188)]
			startOfPayload := uint8(4)
			payloadLength  := uint8(184)
			tsAdaptFields := new(tsAdaptInfo)
			header := parseTSHeader (nextPacket)
			pidData := metaInfo.pidStats[header.pid]
			

			if (header.adaptation & 0x2) == 0x2 {
				adaptationLength := uint8(nextPacket[4])
				if adaptationLength != 0 {
					adaptationBitField := uint8(nextPacket[5])
					parseTSAdaptFields(adaptationBitField, tsAdaptFields)
					startOfPayload += (1 + adaptationLength);
					payloadLength = 184 - (1 + adaptationLength);

					if tsAdaptFields.pcrFlag == 1 {
						pcr27Mhz := extractPCR(nextPacket[5:5+adaptationLength])
						fmt.Printf("%v", pcr27Mhz)
					}
				}
			}
			
			if pidData.packetCount!= 0 {
				expectedContCount := pidData.lastContCount
				if (header.adaptation & 0x1) == 0x1 {
					expectedContCount = (expectedContCount + 1) & 0xf;
				}
				if ((expectedContCount != header.contCount) && (tsAdaptFields.discontinuityFlag == 0)) {
					pidData.contCountErrors += 1
				}
			}
			pidData.lastContCount = header.contCount
			pidData.packetCount += 1
			
			if header.pid == 0 {
				fmt.Printf("\n[%v] %v %v %v",header.pid, payloadLength, startOfPayload, header.adaptation)
			}
			metaInfo.tables.checkForSiPsi(header.pid, header.payloadUnitStart, payloadLength, nextPacket[startOfPayload:] )

			metaInfo.globalStats.totalPackets += 1
			metaInfo.pidStats[header.pid] = pidData

			//fmt.Printf(" sync 0x%x  payloadLength %v", header.syncByte, payloadLength)
		}
	}
	return
}