package enums

type Target int

const (
	Direct    Target = 0
	Spread    Target = 1
	Broadcast Target = 2
)

type Message string

const (
	Submit     Message = "SUBMIT"
	SubmitAck  Message = "SUBMIT_ACK"
	Bid        Message = "BID"
	BidAck     Message = "BID_ACK"
	BidReject  Message = "BID_REJECT"
	JobSuccess Message = "JOB_SUCCESS"
	JobFailure Message = "JOB_FAILURE"
	Cancel     Message = "CANCEL"
)

const Tcp string = "tcp"
