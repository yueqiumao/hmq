/* Copyright (c) 2018, joy.zhou <chowyu08@gmail.com>
 */
package broker

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
	"go.uber.org/zap"
)

const (
	Connected    = 1
	Disconnected = 2
)

type client struct {
	mu         sync.Mutex
	broker     *Broker
	conn       net.Conn
	info       info
	status     int
	ctx        context.Context
	cancelFunc context.CancelFunc
	subMap     map[string]*subscription
	topicsMgr  *memTopics
	subs       []interface{}
	qoss       []byte
}

type subscription struct {
	client *client
	topic  string
	qos    byte
}

type info struct {
	clientID  string
	username  string
	password  []byte
	keepalive uint16
	willMsg   *packets.PublishPacket
	localIP   string
	remoteIP  string
}

var (
	DisconnectdPacket = packets.NewControlPacket(packets.Disconnect).(*packets.DisconnectPacket)
)

func (c *client) init() {
	c.status = Connected
	c.info.localIP = strings.Split(c.conn.LocalAddr().String(), ":")[0]
	c.info.remoteIP = strings.Split(c.conn.RemoteAddr().String(), ":")[0]
	c.ctx, c.cancelFunc = context.WithCancel(context.Background())
	c.subMap = make(map[string]*subscription)
	c.topicsMgr = c.broker.topicsMgr
}

func (c *client) readLoop() {
	nc := c.conn
	b := c.broker
	if nc == nil || b == nil {
		return
	}

	keepAlive := time.Second * time.Duration(c.info.keepalive)
	timeOut := keepAlive + (keepAlive / 2)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			//add read timeout
			if err := nc.SetReadDeadline(time.Now().Add(timeOut)); err != nil {
				log.Print("set read timeout error: ", err, zap.String("ClientID", c.info.clientID))
				msg := &Message{
					client: c,
					packet: DisconnectdPacket,
				}
				ProcessMessage(msg)
				return
			}

			packet, err := packets.ReadPacket(nc)
			if err != nil {
				log.Print("read packet error: ", err, zap.String("ClientID", c.info.clientID))
				msg := &Message{
					client: c,
					packet: DisconnectdPacket,
				}
				ProcessMessage(msg)
				return
			}

			msg := &Message{
				client: c,
				packet: packet,
			}
			ProcessMessage(msg)
		}
	}

}

func ProcessMessage(msg *Message) {
	c := msg.client
	ca := msg.packet
	if ca == nil {
		return
	}

	switch ca.(type) {
	case *packets.ConnackPacket:
	case *packets.ConnectPacket:
	case *packets.PublishPacket:
		packet := ca.(*packets.PublishPacket)
		c.processClientPublish(packet)
	case *packets.PubackPacket:
	case *packets.PubrecPacket:
	case *packets.PubrelPacket:
	case *packets.PubcompPacket:
	case *packets.SubscribePacket:
		packet := ca.(*packets.SubscribePacket)
		c.processClientSubscribe(packet)
	case *packets.SubackPacket:
	case *packets.UnsubscribePacket:
		packet := ca.(*packets.UnsubscribePacket)
		c.processClientUnSubscribe(packet)
	case *packets.UnsubackPacket:
	case *packets.PingreqPacket:
		c.ProcessPing()
	case *packets.PingrespPacket:
	case *packets.DisconnectPacket:
		c.Close()
	default:
		log.Print("Recv Unknow message.......", zap.String("ClientID", c.info.clientID))
	}
}

func (c *client) processClientPublish(packet *packets.PublishPacket) {
	switch packet.Qos {
	case QosAtMostOnce:
		c.ProcessPublishMessage(packet)
	case QosAtLeastOnce:
		puback := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
		puback.MessageID = packet.MessageID
		if err := c.WriterPacket(puback); err != nil {
			log.Print("send puback error, ", err, zap.String("ClientID", c.info.clientID))
			return
		}
		c.ProcessPublishMessage(packet)
	case QosExactlyOnce:
		return
	default:
		log.Print("publish with unknown qos", zap.String("ClientID", c.info.clientID))
		return
	}

}

func (c *client) ProcessPublishMessage(packet *packets.PublishPacket) {

	b := c.broker
	if b == nil {
		return
	}

	err := c.topicsMgr.Subscribers([]byte(packet.TopicName), packet.Qos, &c.subs, &c.qoss)
	if err != nil {
		// log.Error("Error retrieving subscribers list: ", zap.String("ClientID", c.info.clientID))
		return
	}

	// fmt.Println("psubs num: ", len(c.subs))
	if len(c.subs) == 0 {
		return
	}

	for _, sub := range c.subs {
		s, ok := sub.(*subscription)
		if ok {
			publish(s, packet)
		}
	}
}

func (c *client) processClientSubscribe(packet *packets.SubscribePacket) {
	if c.status == Disconnected {
		return
	}

	b := c.broker
	if b == nil {
		return
	}
	topics := packet.Topics
	qoss := packet.Qoss

	suback := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	suback.MessageID = packet.MessageID
	var retcodes []byte

	for i, topic := range topics {
		t := topic

		if oldSub, exist := c.subMap[t]; exist {
			c.topicsMgr.Unsubscribe([]byte(oldSub.topic), oldSub)
			delete(c.subMap, t)
		}

		sub := &subscription{
			topic:  topic,
			qos:    qoss[i],
			client: c,
		}

		rqos, err := c.topicsMgr.Subscribe([]byte(topic), qoss[i], sub)
		if err != nil {
			log.Print("subscribe error, ", err, zap.String("ClientID", c.info.clientID))
			retcodes = append(retcodes, QosFailure)
			continue
		}

		c.subMap[t] = sub

		retcodes = append(retcodes, rqos)

	}

	suback.ReturnCodes = retcodes

	err := c.WriterPacket(suback)
	if err != nil {
		log.Print("send suback error, ", err, zap.String("ClientID", c.info.clientID))
		return
	}
}

func (c *client) processClientUnSubscribe(packet *packets.UnsubscribePacket) {
	if c.status == Disconnected {
		return
	}
	b := c.broker
	if b == nil {
		return
	}
	topics := packet.Topics

	for _, topic := range topics {
		sub, exist := c.subMap[topic]
		if exist {
			c.topicsMgr.Unsubscribe([]byte(sub.topic), sub)
			delete(c.subMap, topic)
		}
	}

	unsuback := packets.NewControlPacket(packets.Unsuback).(*packets.UnsubackPacket)
	unsuback.MessageID = packet.MessageID

	err := c.WriterPacket(unsuback)
	if err != nil {
		log.Print("send unsuback error, ", err, zap.String("ClientID", c.info.clientID))
		return
	}
}

func (c *client) ProcessPing() {
	if c.status == Disconnected {
		return
	}
	resp := packets.NewControlPacket(packets.Pingresp).(*packets.PingrespPacket)
	err := c.WriterPacket(resp)
	if err != nil {
		log.Print("send PingResponse error, ", err, zap.String("ClientID", c.info.clientID))
		return
	}
}

func (c *client) Close() {
	if c.status == Disconnected {
		return
	}

	c.cancelFunc()

	c.status = Disconnected

	b := c.broker
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	subs := c.subMap

	if b != nil {
		b.removeClient(c)
		for _, sub := range subs {
			err := b.topicsMgr.Unsubscribe([]byte(sub.topic), sub)
			if err != nil {
				log.Print("unsubscribe error, ", err, zap.String("ClientID", c.info.clientID))
			}
		}

		if c.info.willMsg != nil {
			b.PublishMessage(c.info.willMsg)
		}
	}
}

func (c *client) WriterPacket(packet packets.ControlPacket) error {
	if c.status == Disconnected {
		return nil
	}

	if packet == nil {
		return nil
	}
	if c.conn == nil {
		c.Close()
		return errors.New("connect lost ....")
	}

	c.mu.Lock()
	err := packet.Write(c.conn)
	c.mu.Unlock()
	return err
}
