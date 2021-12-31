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
    tsProgramMapSection tableIDsEnum = 0x02
    sdtSectionActualTransportStream tableIDsEnum = 0x42
	scte35SpliceInfoSection tableIDsEnum = 0xfc
)


func (tableID tableIDsEnum) String() string {
	switch tableID {
	case programAssociationSection:
		return "programAssociationSection"
	case tsProgramMapSection:
		return "tsProgramMapSection"
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