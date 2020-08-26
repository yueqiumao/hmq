/* Copyright (c) 2018, joy.zhou <chowyu08@gmail.com>
 */
package broker

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/fhmq/hmq/broker/lib/topics"
	"golang.org/x/net/websocket"
)

type Message struct {
	client *client
	packet packets.ControlPacket
}

type Broker struct {
	id        string
	mu        sync.Mutex
	config    *Config
	clients   sync.Map
	topicsMgr *topics.Manager
	// sessionMgr *sessions.Manager
	// auth       auth.Auth
}

func NewBroker(config *Config) (*Broker, error) {
	if config == nil {
		config = DefaultConfig
	}

	b := &Broker{
		id:     GenUniqueId(),
		config: config,
	}

	var err error
	b.topicsMgr, err = topics.NewManager("mem")
	if err != nil {
		// log.Error("new topic manager error", err)
		return nil, err
	}

	// b.sessionMgr, err = sessions.NewManager("mem")
	// if err != nil {
	// 	// log.Error("new session manager error", err)
	// 	return nil, err
	// }

	//b.auth = auth.NewAuth(b.config.Plugin.Auth)

	return b, nil
}

func (b *Broker) Start() {
	if b == nil {
		// log.Error("broker is null")
		return
	}

	//listen clinet over tcp
	if b.config.Port != "" {
		go b.StartClientListening()
	}

	//listen for websocket
	if b.config.WsPort != "" {
		go b.StartWebsocketListening()
	}
}

func (b *Broker) StartWebsocketListening() {
	path := b.config.WsPath
	hp := ":" + b.config.WsPort
	// log.Info("Start Websocket Listener on:", zap.String("hp", hp), zap.String("path", path))
	http.Handle(path, websocket.Handler(b.wsHandler))
	err := http.ListenAndServe(hp, nil)
	if err != nil {
		// log.Error("ListenAndServe:" + err.Error())
		return
	}
}

func (b *Broker) wsHandler(ws *websocket.Conn) {
	// io.Copy(ws, ws)
	ws.PayloadType = websocket.BinaryFrame
	b.handleConnection(ws)
}

func (b *Broker) StartClientListening() {
	var hp string
	var err error
	var l net.Listener
	hp = b.config.Host + ":" + b.config.Port
	l, err = net.Listen("tcp", hp)
	// log.Info("Start Listening client on ", zap.String("hp", hp))
	if err != nil {
		// log.Error("Error listening on ", err)
		return
	}
	tmpDelay := 10 * ACCEPT_MIN_SLEEP
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				// log.Error("Temporary Client Accept Error(%v), sleeping %dms",
				// zap.Error(ne), zap.Duration("sleeping", tmpDelay/time.Millisecond))
				time.Sleep(tmpDelay)
				tmpDelay *= 2
				if tmpDelay > ACCEPT_MAX_SLEEP {
					tmpDelay = ACCEPT_MAX_SLEEP
				}
			} else {
				// log.Error("Accept error: %v", err)
			}
			continue
		}
		tmpDelay = ACCEPT_MIN_SLEEP
		go b.handleConnection(conn)

	}
}

func (b *Broker) handleConnection(conn net.Conn) {
	//process connect packet
	packet, err := packets.ReadPacket(conn)
	if err != nil {
		// log.Error("read connect packet error: ", err)
		return
	}
	if packet == nil {
		// log.Error("received nil packet")
		return
	}
	msg, ok := packet.(*packets.ConnectPacket)
	if !ok {
		// log.Error("received msg that was not Connect")
		return
	}

	// log.Info("read connect from ", zap.String("clientID", msg.ClientIdentifier))

	connack := packets.NewControlPacket(packets.Connack).(*packets.ConnackPacket)
	connack.SessionPresent = msg.CleanSession
	connack.ReturnCode = msg.Validate()

	if connack.ReturnCode != packets.Accepted {
		err = connack.Write(conn)
		if err != nil {
			// log.Error("send connack error, ", err, zap.String("clientID", msg.ClientIdentifier))
			return
		}
		return
	}

	// if !b.CheckConnectAuth(string(msg.ClientIdentifier), string(msg.Username), string(msg.Password)) {
	// 	connack.ReturnCode = packets.ErrRefusedNotAuthorised
	// 	err = connack.Write(conn)
	// 	if err != nil {
	// 		// log.Error("send connack error, ", err, zap.String("clientID", msg.ClientIdentifier))
	// 		return
	// 	}
	// 	return
	// }

	err = connack.Write(conn)
	if err != nil {
		// log.Error("send connack error, ", err, zap.String("clientID", msg.ClientIdentifier))
		return
	}

	willmsg := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	if msg.WillFlag {
		willmsg.Qos = msg.WillQos
		willmsg.TopicName = msg.WillTopic
		willmsg.Retain = msg.WillRetain
		willmsg.Payload = msg.WillMessage
		willmsg.Dup = msg.Dup
	} else {
		willmsg = nil
	}
	info := info{
		clientID:  msg.ClientIdentifier,
		username:  msg.Username,
		password:  msg.Password,
		keepalive: msg.Keepalive,
		willMsg:   willmsg,
	}

	c := &client{
		broker: b,
		conn:   conn,
		info:   info,
	}

	c.init()

	// err = b.getSession(c, msg, connack)
	// if err != nil {
	// 	// log.Error("get session error: ", zap.String("clientID", c.info.clientID))
	// 	return
	// }

	cid := c.info.clientID

	var exist bool
	var old interface{}
	old, exist = b.clients.Load(cid)
	if exist {
		// log.Warn("client exist, close old...", zap.String("clientID", c.info.clientID))
		ol, ok := old.(*client)
		if ok {
			ol.Close()
		}
	}
	b.clients.Store(cid, c)

	c.readLoop()
}

func (b *Broker) removeClient(c *client) {
	clientId := string(c.info.clientID)
	b.clients.Delete(clientId)
	// log.Info("delete client ,", clientId)
}

func (b *Broker) PublishMessage(packet *packets.PublishPacket) {
	var subs []interface{}
	var qoss []byte
	b.mu.Lock()
	err := b.topicsMgr.Subscribers([]byte(packet.TopicName), packet.Qos, &subs, &qoss)
	b.mu.Unlock()
	if err != nil {
		// log.Error("search sub client error,  ", err)
		return
	}

	for _, sub := range subs {
		s, ok := sub.(*subscription)
		if ok {
			err := s.client.WriterPacket(packet)
			if err != nil {
				// log.Error("write message error,  ", err)
			}
		}
	}
}
