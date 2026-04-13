package centrifugo

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/gocent/v3"
)

type publisher struct {
	client *gocent.Client
}

func NewPublisher(client *gocent.Client) *publisher {
	return &publisher{client: client}
}

func (p *publisher) PublishMessage(ctx context.Context, channel string, data interface{}) error {
	bytesData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = p.client.Publish(ctx, channel, bytesData)
	return err
}
