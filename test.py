# -*- coding: utf-8 -*-    
import paho.mqtt.client as mqtt  
import random

# 连接成功 (重连也会进这里)
def on_connect(client, userdata, flags, rc):  
    print("Connected with result code " + str(rc))  
    # 订阅主题
    client.subscribe("#")  
    # 发送消息
    client.publish("hello/world", b"hello...")
  
# 收到消息
def on_message(client, userdata, msg):  
    print(msg.topic + " " + str(msg.payload))  

def ranstr(num):
    # 猜猜变量名为啥叫 H
    H = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'

    salt = ''
    for i in range(num):
        salt += random.choice(H)

    return salt

client = mqtt.Client(client_id=ranstr(6))  
client.on_connect = on_connect  
client.on_message = on_message  

try:  
    # 连接服务器
    client.connect("localhost", 1883)  
    client.loop_forever()  
except KeyboardInterrupt:  
    client.disconnect()  