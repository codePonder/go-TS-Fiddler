// this package provides support for mpeg2 ts functions. demuxing from file, checking the 4 byte header, pcr checking, PAT/ PMT / SDT and SCTE-35 parsing
package tshelper

import (
	"errors"
	"fmt"
)

// the data structure that is the TS-Demultiplxer
type tsdmx struct {
	pidStats map[uint16]pidInfo
	globalStats *globalInfo
	tables tableParser
}

// information on what we have seen on individual PIDs
type pidInfo struct {
	lastContCount uint8
	contCountErrors uint64
	packetCount uint64
	bitrate uint64
	packetCountSinceBitrateCalc uint64
	pcr27MHzAtLastBitrateSlice uint64
	pktCountAtLastBitrateSlice uint64
}


// information on what we have seen generic to the wholestream
// for a crude measure of bitrate, just pick 1 PCR PID (any will do :-) )
// and then count pkts on each pid seen between PCR packets
type globalInfo struct {
	totalPackets uint64
	pcrUsedForCrudeTimings uint16
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
	newStruct.globalStats = new(globalInfo)
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


// TODO - make a more precise measure of bitrate.  For now, its enough to just...
// PCRs give us a measure of elapsed time in a TS file, there isn't any other reference in a raw TS File
// Each program has its own PCR, but.... all we really need is a measure of time and a number of packets to 
// estimate the bitrate of a PID component.  Yes using the PCR of the service would be more accurate, but for
// _most_ purposes its enough to just have _a_ measure of elapsed time that is close enough
func takeBitrateSlice(pidStats *map[uint16]pidInfo, pcrNow uint64) {

		for pid, info := range (*pidStats) {
			packetDelta := info.packetCount - info.pktCountAtLastBitrateSlice
			// TODO - PCRs wrap aouund ~ 26hours - cope with it!!!!!
			pcrDelta27Mhz := pcrNow - info.pcr27MHzAtLastBitrateSlice
			bitrate := ((188 * 8 * packetDelta) * 27000000) / (pcrDelta27Mhz + 1)
			fmt.Printf("\n  PID known 0x%x  rate %v", pid, bitrate)
			info.pktCountAtLastBitrateSlice = info.packetCount
			info.bitrate = bitrate
			info.pcr27MHzAtLastBitrateSlice = pcrNow
			(*pidStats)[pid] = info
		}
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
					if tsAdaptFields.pcrFlag != 0 {
						pcr27Mhz := extractPCR(nextPacket[6:6+adaptationLength])
						fmt.Printf(" \n PCR %v   at count %d   %d 0x%x %d", pcr27Mhz, pidData.packetCount, metaInfo.globalStats.pcrUsedForCrudeTimings, header.pid, 	metaInfo.globalStats.totalPackets )
						if metaInfo.globalStats.pcrUsedForCrudeTimings == 0 {
							metaInfo.globalStats.pcrUsedForCrudeTimings = header.pid
						} else if metaInfo.globalStats.pcrUsedForCrudeTimings == header.pid {
							takeBitrateSlice(&metaInfo.pidStats, pcr27Mhz)
							pidData = metaInfo.pidStats[header.pid]
						}
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
			
			metaInfo.tables.checkForSiPsi(header.pid, header.payloadUnitStart, payloadLength, nextPacket[startOfPayload:] )

			metaInfo.globalStats.totalPackets += 1
			metaInfo.pidStats[header.pid] = pidData

			//fmt.Printf(" sync 0x%x  payloadLength %v", header.syncByte, payloadLength)
		}
	}
	return
}


// summarise what structures have been found
func (metaInfo tsdmx) SummariseFindings() {

	fmt.Printf("\n ###################### \n")
	for k := range metaInfo.pidStats {
        fmt.Printf("PID found 0x%x    pkts %d \n", k, metaInfo.pidStats[k].packetCount)
    }
	metaInfo.tables.summariseServiceList()
	fmt.Printf(" \n Total Packets seem %d\n ", metaInfo.globalStats.totalPackets)
	fmt.Printf(" ###################### \n")
	

}