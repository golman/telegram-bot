package main

type MediaMessage struct {
	fileid            []string
	username          string
	fullname          string
	userid            int64
	originalmessageid int
	caption           string
	state             MessageState
}

type MessageState int

const (
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	MessageStateToDelete
)

func (mm *MediaMessage) createCaption() string {
	return createCaption(mm.caption, mm.fullname, mm.userid)
}
