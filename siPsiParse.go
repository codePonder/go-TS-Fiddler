package tshelper

// contains the logic to process basic tables
// PAT
// PMT
// SDT 
// SCTE-35 tables

// does not yet cope with tables that span multiple TS packets

import (
	"fmt"
)

type tableTypeEnum uint8
type tableIDsEnum uint8

 // from Table 2-31 – table_id assignment values in the mpeg systems spec
const (
    programAssociationSection tableIDsEnum = 0x00
    ProgramMapSection tableIDsEnum = 0x02
    sdtSectionActualTransportStream tableIDsEnum = 0x42
	scte35SpliceInfoSection tableIDsEnum = 0xfc
)


func (tableID tableIDsEnum) String() string {
	switch tableID {
	case programAssociationSection:
		return "programAssociationSection"
	case ProgramMapSection:
		return "ProgramMapSection"
	case sdtSectionActualTransportStream:
		return "sdtSectionActualTransportStream"
	case scte35SpliceInfoSection:
		return "scte35SpliceInfoSection"
	}
	return "unknown tableID"
}

const (
	patTable tableTypeEnum = iota
	sdtTable
	pmtTable
	nitTable
	scte35Table
)

func (tableType tableTypeEnum) String() string {
	switch tableType {
	case patTable:
		return "patTable"
	case sdtTable:
		return "sdtTable"
	case pmtTable:
		return "pmtTable"
	case nitTable:
		return "nitTable"
	case scte35Table:
		return "scte35Table"
	}
	return "unknown"
}


type streamComponentDefinition struct {
	streamType uint8
	streamPID uint16
	cueDescriptor bool
}

type programDefinition struct{
	pcrPID uint16
	programNumber uint16
	programHasSCTE35 bool
	definedMaxBitrate uint32
	numberOfStreams uint32
	streamComps []streamComponentDefinition
}

type pmtInfoStored struct {
    streamType uint8
    streamPID uint16
    cueDescriptor bool
}

type tablesMapEntry struct {
	tabletype tableTypeEnum
	programNumber uint16
	latestVersion uint8
	versionsSeen uint64
	numberTablesSeen uint64
	isSingleSection bool
}


// structure that is the tableParser
type tableParser struct {
	tablesMap map[uint16]tablesMapEntry
}



func newTableParser () tableParser {
	newStruct := tableParser {}
	newStruct.tablesMap = make(map[uint16]tablesMapEntry)
	
	piddata := tablesMapEntry {}

	piddata.tabletype = patTable
	newStruct.tablesMap[0x00] = piddata
	
	piddata.tabletype = sdtTable
	newStruct.tablesMap[0x11] = piddata
	
	return newStruct
}


// tests if the packet is in the list of "known" siPsi
// process is
// at time 0, just know of PATs on PID 0 & SDT on PID 0x12- find these
// PAT leads to PMTs
// until PAT is parsed, you cannot find PMTs as their PID varies
// checkForSiPsi is entered pointing at the start of the data just past
// where the adaptation field data ended.  As long as the PUSI bit is set then
// a section starts in this TS packet.  IF so, first byte is the payloadOffset.
// read this, jump to where it points and start parsing
//  tableID :8
//  sectionlength :16  (bottom 12 actually )
//	transportStreamID : 16
// 	versionNumber : 7 (bottom 5)
//	currentNext : 1
// 	sectionNumber : 8
// 	lastSectionNumber : 8  
// after this lot - can call specific parsers

// TODO - handle tables that span multiple sections 
// TODO - tables can span mutiple packets, this is not covered YET

func(tables tableParser) checkForSiPsi(pid uint16, pusi uint8, dataLeft uint8, data[]byte) {

	_, isTable := tables.tablesMap[pid];

	if isTable {
		if pusi == 1 {

			pointerField := uint8(data[0]) + 1
			activeData := data[pointerField:]
			dataLeft -= pointerField

			tableID := tableIDsEnum(activeData[0])
			sectionLength := ((uint16(activeData[1]) << 8 ) + uint16(activeData[2]) ) & 0x0fff

			if (sectionLength <= uint16(dataLeft)) {
				// transportStreamID 3, 4
				// versionNumber := (uint8(activeData[5]) >> 1) & 0x1f
				// currentNext := (uint8(activeData[5])) & 0x1
				//sectionNumber := uint8(activeData[6])
				//lastSectionNumber := (uint8(activeData[7]))
				sectionLength -= 5
				dataLeft -= 8
				if tableID == programAssociationSection {
					 patParser (activeData[8:], sectionLength, tables.tablesMap)
					 fmt.Printf("%v \n", tables.tablesMap)
				} else if tableID == ProgramMapSection{
					 pmtParser (activeData[8:], sectionLength)
				} else if tableID == sdtSectionActualTransportStream {
					sdtParser (activeData[8:], sectionLength)
			   }
			} else {
				fmt.Printf("\n [%v] %v %v Table Section Length %v > payload available %v in 1 packet - NOT SUPORTED", pid, pusi, tableID,  sectionLength, dataLeft )
			}
		}
	} else {
		fmt.Print(".")
	}
}


// Program Association table
// This is parsed to find the PIDS that the Program Map Table (PMT) can be found
// for the services in this stream.
// jump here after the tavle upto and including last_section_number
// TODO  - PATs have a CRC that should be checked for validty.... 
func patParser (dataBuffer []byte, dataLeft uint16, tableMap  map[uint16]tablesMapEntry) {
	
	for rd := 0 ; dataLeft > 4; dataLeft -= 4 {
		programNumber := (uint16(dataBuffer[rd]) << 8) + uint16(dataBuffer[rd+1])
		pid := ((uint16(dataBuffer[rd+2]) << 8) + uint16(dataBuffer[rd+3])) & 0x1fff
		rd += 4
		// overwrite or create if needed an entry in Tables for PMT just referenced
		tableEntry := tableMap[pid]
		if programNumber == 0 {
			tableEntry.tabletype = nitTable
		} else {
			tableEntry.tabletype = pmtTable
			fmt.Printf(" \n From PAT :: PMT prog # %d  is 0x%x \n", programNumber, pid)
		}
		tableEntry.programNumber = programNumber
		tableMap[pid] = tableEntry
	}

	// TODO add in a CRC Check here to make sure PAT is valid
}

// Program Map Table Parsing
// The program map is, a map of what PIDs provide components in the program
// contains descriptors of what "type" services are, PIDs to locate and a PCR reference
// TODO - really should be checking the CRC, tracking version and handling multi-section PMTs
// This initial code is only meant for use with SIMPLE streams where the PMT fits in 1 TS packet

func pmtParser (dataBuffer []byte, dataLeft uint16) {

	programContainsSCTE35 := false
	maxBitrate := uint32(0) 
	serviceID := uint16(1) // progInfo.programNumber;
	pcrPID 	:= ( (uint16(dataBuffer[0]) << 8) | uint16(dataBuffer[1]) ) & 0x1fff
	programInfoLength := ( (uint16(dataBuffer[2]) << 8) | uint16(dataBuffer[3]) ) & 0x0fff
	dataLeft -= 4

	descriptorDataLeft :=  programInfoLength
	nextDescriptorStart := 4  	// we start the first desriptor after using 4 bytes of data passed in
	progInfoBuffer := dataBuffer[(programInfoLength + 4):]
	
	// first get the program level descriptors
	if programInfoLength != 0 {
		for descriptorDataLeft != 0 {
			rd := nextDescriptorStart
			tag := uint8(dataBuffer[rd])
			length := uint8(dataBuffer[rd+1])
			rd += 2
			nextDescriptorStart += int(length + 2) // +2 as jumping body + tag +length field
			descriptorDataLeft  -= uint16(length + 2)
			if (tag == 5) {
				if (length == 4) {
					id := ( (uint32(dataBuffer[rd+0]) << 24) |
							(uint32(dataBuffer[rd+1]) << 16) |
							(uint32(dataBuffer[rd+2]) <<  8) |
							(uint32(dataBuffer[rd+3]) <<  0) )
					if id == 0x43554549 {
						programContainsSCTE35 = true;
						fmt.Println(" SCTE-35 descriptor seen")
					}
				} else {
					fmt.Println(" SCTE-35 Tag found, length wrong") // TODO raise error
				}
			} else if tag == 14 {
				if (length == 0x3) {

					maxBitrate := (( (uint32(dataBuffer[rd+0]) << 16) |
									 (uint32(dataBuffer[rd+1]) <<  8) |
									 (uint32(dataBuffer[rd+2]) <<  0) ) & 0x3fffff ) * 50*8
					fmt.Printf(" \n MaxBitrate descriptor %d \n",  maxBitrate)
				} else {
					fmt.Println(" MaxBitrate descriptor  Tag found, length wrong") // TODO raise error
				} 
			}
		}
	}
	// Program Info loop
	dataLeft -= programInfoLength

	// TODO create an interface to craete these structure and make a 0 length list in side, as opposed to needing to do
	// by hand each time
	programDef := new(programDefinition)
	programDef.streamComps = make([]streamComponentDefinition, 0)

	programDef.pcrPID = pcrPID
	programDef.programNumber = serviceID
	programDef.programHasSCTE35  = programContainsSCTE35
	programDef.definedMaxBitrate = maxBitrate
	programDef.numberOfStreams = 0

	streamDef := streamComponentDefinition {}
	rd := 0	

	// TODO - are these structure deepcopied over, or just the pointer in memory ?
	for dataLeft > 4 {

		streamDef.streamType = uint8(progInfoBuffer[rd+0])
		streamDef.streamPID  = ( (uint16(progInfoBuffer[rd+1]) << 8) |
							     (uint16(progInfoBuffer[rd+2]) << 0) ) & 0x1fff
		streamDef.cueDescriptor = false
		dataLeft -= 3

		esInfoLength := ( (uint16(progInfoBuffer[rd+3]) << 8) |
						  (uint16(progInfoBuffer[rd+4]) << 0) ) & 0x0fff
		dataLeft -= 2
		
		rd += 5
		programDef.streamComps = append(programDef.streamComps, streamDef)

		// the only stream type sought is cue descriptors, ie SCTE35 stuff.   Parse through 
		// the data to check for them
		{
			// look for cue descriptors in the elementary stream descriptors
			descriptorLength := esInfoLength;
			descStart := rd
			for descriptorLength > 0 {
				tag := uint8(progInfoBuffer[descStart])
				length := uint8(progInfoBuffer[descStart+1])
		 		if tag == 0x8a {
		 			streamDef.cueDescriptor = true
		 			if programContainsSCTE35 {
						//  TODO store scte35 pid to "look for tables"
		 			}
		 		} 
		 		descStart += int(length)
		 		descriptorLength -= uint16(1 + 1 + length);
		 	}
		}
		rd += int(esInfoLength)
		dataLeft -= esInfoLength;
	}

	// TODO - catch system if number programs is exploding on us

// CRC is last 4 bytes, ignore for now    TODO - handle and check CRCs
}




// Parse the SDT 
func sdtParser (dataBuffer []byte, dataLeft uint16) {

	rd := 0
	rd += 3 //  ignore original_network_id :16 and reserved_future_use :8 
	dataLeft -= 3

	for dataLeft > 4 {
		serviceID := (uint16(dataBuffer[rd]) << 8) | uint16(dataBuffer[rd+1])
		dataLeft -= 2
		// ignore  reserved_future_use :6, EIT_schedule_flag :1  EIT_present_following_flag : 1 
		dataLeft -= 1

		desriptorLength := ( (uint16(dataBuffer[rd+3]) << 8) | uint16(dataBuffer[rd+4])  ) & 0x0fff
		dataLeft -= 2
		
		bytesAvailable := int(desriptorLength)
		descriptorRd := rd + 5

		for bytesAvailable > 0 {
			descriptorTag := uint8(dataBuffer[descriptorRd])
			length := uint8(dataBuffer[descriptorRd + 1])

			if descriptorTag == 0x48 {
				serviceProviderNameLength := uint8(dataBuffer[descriptorRd + 3]) // ignore service type - so +3
				serviceNameLength := uint8(dataBuffer[descriptorRd + 3 + 1 + int(serviceProviderNameLength)])
				serviceNameStart := (descriptorRd + 3 + 1 + int(serviceProviderNameLength) + 1 )
				serviceName := string( dataBuffer[serviceNameStart : serviceNameStart + int(serviceNameLength) ] )
				fmt.Printf("\n SDT : [%d] :: %s  ",serviceID,  serviceName)
			}
			descriptorRd += int(length + 2)
			bytesAvailable -= int(length + 2)

		}

		rd += int(desriptorLength)
		dataLeft -= uint16(desriptorLength)
	}

	//TODO HANDLE THE crc 

}
