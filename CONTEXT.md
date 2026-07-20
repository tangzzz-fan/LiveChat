# LiveChat

LiveChat 是一个面向即时通信系统工程学习的单上下文项目，核心关注消息正确性、长连接接入、离线同步、多端一致性、群聊扇出、媒体消息、推送与安全边界。

## Language

**User**:
聊天系统中的账号主体，代表一个可登录、可发消息、可加入会话的人。
_Avoid_: Account holder, profile

**Device**:
用户登录态承载单元，代表一个具体的 iPhone、Android 设备或桌面端实例。
_Avoid_: Client instance, terminal

**Conversation**:
消息收发的逻辑容器，可以是单聊或群聊。
_Avoid_: Chat room, thread

**Message**:
会话中的一条业务消息，具备客户端幂等标识、服务端主键和会话内顺序语义。
_Avoid_: Packet, payload

**MessageReceipt**:
描述消息送达或已读推进的状态确认记录，不等同于消息实体本身。
_Avoid_: Ack record, status row

**Attachment**:
挂载在消息上的媒体或文件元数据，内容本体通常位于对象存储而不是消息主表。
_Avoid_: Blob, file body

**SyncCursor**:
用于离线补拉和断点续同步的位置游标，表示客户端已消费到哪里的事实。
_Avoid_: Offset, checkpoint

**Outbox**:
客户端本地发送队列，负责承接弱网、重试与幂等发送，不直接代表服务端持久化成功。
_Avoid_: Send buffer, pending list

**Gateway**:
长连接接入层，负责 WebSocket 连接、鉴权后会话建立、心跳和协议转发，不承载消息业务语义。
_Avoid_: Chat server, message server

**Message Service**:
负责消息校验、持久化、排序、扇出和同步事件生成的核心业务服务。
_Avoid_: Gateway, transport layer

**Delivery**:
消息从服务端投递到目标设备的异步阶段，只表示设备侧收到投递，不代表用户已读。
_Avoid_: Acceptance, persistence

**Read Position**:
会话内已读推进位置，通常通过单调递增序号表达，而不是逐条消息布尔值。
_Avoid_: Read flag, message-level read state
