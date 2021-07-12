# Ultraviolet
## What is it
Like [Infrared](https://github.com/haveachin/infrared), Ultraviolet is an ultra lightweight Minecraft reverse proxy written in Go. Not even sure or this will be a production ready software some day, its mostly a different structure I want to try and see what kind of effect it has on performance and such. It should work most of the time, although there isnt much code/features dedicated to prevent some mistakes from happening or which can recover when certain errors occur.


## Some notes
### How to build
Ultraviolet can be ran by using docker or you can also build a binary yourself by running:
```
go build -tags netgo
```  

### Features
[x] HAProxy protocol(v2) support (sending only)  
[x] Can restart without shutting down open connections (check [Tableflip](#tableflip))  
[x] Rate limiting  
[x] Status caching (online status only)  
[x] Offline status placeholder  
[ ] Anti bot   
... More coming later


### Im not gonna fool proof this
Im not planning on writing code to prevent Ultraviolet from crashing if you did something stupid. If the config wants a timeout time and you put in a negative number that may or may not cause some issues and that is your own fault. 

### Tableflip
This has implemented [tableflip](https://github.com/cloudflare/tableflip) which should make it able to reload/restart Ultraviolet without closing existing connections on Linux and macOS. Ultraviolet should still be usable on windows (testing purposes only pls). 
Check their [documentation](https://pkg.go.dev/github.com/cloudflare/tableflip) to know what or how. 

IMPORTANT: There is a limit of one 'parent' process. So when you reload Ultraviolet once you need to wait until the parent process is closed (all previous connections have been closed) before you can reload it again. 

## Command-Line Flags
`-pid-file` specifies the path of the pid file ultraviolet will use [default: `"/run/ultraviolet.pid"`]

`-config` specifies the path to the main config [default: `"/etc/ultraviolet/ultraviolet.json"`]

`-server-configs` specifies the path to all your server configs [default: `"/etc/ultraviolet/config/"`]


## How does some stuff work
### rate limiting
With rate limiting Ultraviolet will allow a specific number of connections to be made to the backend within a given time frame. It will reset when the time frame `rateCooldown` has passed. When the number has been exceeded but the cooldown isnt over yet, Ultraviolet will behave like the server is offline. Unless cache status has been turned on. Then it would send the cached status of the server if the state of the server is `ONLINE`. Disabling rate limiting can be done by setting it to 0 and allows as many connections as it can to be created. (There is no difference in rate limit disonnect and offline disconnect packets yet. This will come when anti bot stuff is being added.)

### state update Cooldown
To prevent a lot of calls being made to the backend without a reason Ultraviolet will keep track of the state from the backend. The state is currently being based on whether or not the backend will accept an tcp connection or not. When this happened and ultraviolet knows that the backend is `ONLINE` or `OFFLINE` it will wait the time the `stateUpdateCooldown` before it will set the state of the server back to `UNKNOWN`. 


## Config
time config values are based on go's duration formatting, valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". They can be used in combination with each other "1m30s".

### Ultraviolet Config
|Field name|Default | Description| 
|:---:|:---:|:---|
|listenTo|-|The address Ultraviolet will listen to for receiving connections.|
|defaultStatus|[this](#status-config-value)|The status Ultraviolet will send to callers when it receives a status handshake where the server address header isnt recognized.|
|numberOfWorkers|0|The number of name resolve workers Ultraviolet will have running, 0 will disabled it and makes is so that the proxy will receive and accept connections but it wont proxy or respond to them. 1 Should be able to handle quite a few connections in a short amount of time but in case more is necessary its possible to increase it. |


### Server Config
- All config values left blank will result into their default value. For example if you dont have `"rateLimit": 5` inside your json, it will automatically put it on 0 which will also disable rate limit.  
- Inside the examples folder there is example of a server config file and the ultraviolet config file. 
- If its a place where you can use an ipv4, ipv6 should also work as well. Not specifying an ip and only using `:25565` will/might end up using both 

|Field name|Default | Description| 
|:---:|:---:|:---|
|domains|-|Place in here all urls which should be used by clients to target the backend.|
|proxyTo|-|It will call this ip/url when its creating a connection to the server.|
|proxyBind|-|The ip it should be using while connection to the server. If it cant use the given value it will fail and the connection wont be created.|
|dialTimeout|1s|Timeout is the maximum amount of time a dial will wait for a connect to complete.|
|sendProxyProtocol|false|Whether or not it should send a ProxyProtocolv2 header to the target.|
|disconnectMessage|-|The message a user will get when its tries to connect to a offline server|
|offlineStatus|[this](#status-config-value)|The status it will send the player when the server is offline.|
|rateLimit|0|The number of connections it will allow to be made to the backend in the given `rateCooldown` time. 0 will disable rate limiting.  |
|rateCooldown|1s|rateCooldown is the time which it will take before the rateLimit will be reset.|
|stateUpdateCooldown|1s|The time it will assume that the state of the server isnt changed (that server isnt offline now while it was online the last time we checked). |
|cacheStatus|false|Turn on or off whether it should cache the online cache of the server. If the server is recognized as `OFFLINE` it will send the offline status to the player.|
|validProtocol|0|validProtocol is the protocol integer the handshake will have when sending the handshake to the backend. Its only necessary to have this when `cacheStatus` is on.|
|cacheUpdateCooldown|1s|The time it will assume that the statys of the server isnt changed (including player count). |


### Status Config value
A status config is build with the following fields
|Field name|Default | Description| 
|:---:|:---:|:---|
|name|-|This is the 'name' of the status response. Its the text which will appear on the left of the latency bar.|
|protocol|0|This is the protocol it will use. For more information about it or to see what numbers belong to which versions check [this website](https://wiki.vg/Protocol_version_numbers) |
|text|-|This is also known as the motd of server.|
|favicon|-|This is the picture it will send to the player. If you want to use this turn the picture you wanna use into a base64 encoded string.|



## Idea notes
### More workers with atomic values
Like how [prometheus does it](https://github.com/prometheus/client_golang/blob/master/prometheus/gauge.go#L82) but than for things like state or rate limit so we can have more workers working on it and increase the load even more. 

### Single liners
- respond in status with the same version the client has sent. 