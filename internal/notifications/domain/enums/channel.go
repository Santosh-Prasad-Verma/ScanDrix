package enums

// Channel represents an alert delivery medium.
type Channel string

const (
	ChannelEmail   Channel = "email"
	ChannelInApp   Channel = "in_app"
	ChannelSlack   Channel = "slack"
	ChannelDiscord Channel = "discord"
	ChannelWebhook Channel = "webhook"
)

// ActiveChannels contains all active delivery channels supported by the platform.
var ActiveChannels = map[Channel]bool{
	ChannelEmail:   true,
	ChannelInApp:   true,
	ChannelSlack:   true,
	ChannelDiscord: true,
	ChannelWebhook: true,
}

// IsActive returns true if the channel is currently enabled and supported.
func (c Channel) IsActive() bool {
	return ActiveChannels[c]
}
