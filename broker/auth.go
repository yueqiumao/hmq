/* Copyright (c) 2018, joy.zhou <chowyu08@gmail.com>
 */
package broker

const (
	SUB = "1"
	PUB = "2"
)

// 检查主题权限
// TODO 区分订阅和发布
func (b *Broker) CheckTopicAuth(action, clientID, username, ip, topic string) bool {
	if b.auth != nil {
		return b.auth.CheckACL(action, clientID, username, ip, topic)
	}
	return true
}

// 检查登陆权限
func (b *Broker) CheckConnectAuth(clientID, username, password string) bool {
	if b.auth != nil {
		return b.auth.CheckConnect(clientID, username, password)
	}
	return true
}
