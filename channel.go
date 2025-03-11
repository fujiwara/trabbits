package trabbits

type Channel struct {
	id   uint16
}

func newChannel(id uint16) *Channel {
	return &Channel{
		id:   id,
	}
}
