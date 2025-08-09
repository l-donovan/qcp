package protocol

const (
	EndTransmission = '\x04'
	FileSeparator   = '\x1c'
	GroupSeparator  = '\x1d'
	RecordSeparator = '\x1e'
	UnitSeparator   = '\x1f'
)

var (
	TerminationSequence = []byte{0x08, 0x08, 0x19, 0x00, 0x20, 0x25}
)
