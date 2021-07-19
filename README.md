# Ultraviolet - Alpha
## Notes
- Tableflip has been removed and all related features  

## What is it
Like [Infrared](https://github.com/haveachin/infrared), Ultraviolet is an ultra lightweight Minecraft reverse proxy written in Go. Not even sure or this will be a production ready software some day, its mostly a different structure I want to try and see what kind of effect it has on performance and such. It should work most of the time, although there isnt much code/features dedicated to prevent some mistakes from happening or which can recover when certain errors occur.


## Some notes
### How to build
Ultraviolet can be ran by using docker or you can also build a binary yourself by running:
```
go build
```  

### Features
[x] HAProxy protocol(v2) support (sending only)  
[x] RealIP (v2.4&v2.5)  
[x] Rate limiting  
[x] Status caching (online status only)  
[x] Offline status placeholder  
... More coming later?


### Im not gonna fool proof this
Im not planning on writing code to prevent Ultraviolet from crashing if you did something stupid. If the config wants a timeout time and you put in a negative number that may or may not cause some issues and that is your own fault. 

## Command-Line Flags
`-config` specifies the path to the main config [default: `"/etc/ultraviolet/ultraviolet.json"`]  
`-server-configs` specifies the path to all your server configs [default: `"/etc/ultraviolet/config/"`]

## How does some stuff work
### rate limiting
With rate limiting Ultraviolet will allow a specific number of connections to be made to the backend within a given time frame. It will reset when the time frame `rateCooldown` has passed. When the number has been exceeded but the cooldown isnt over yet, Ultraviolet will behave like the server is offline.  
By default status request arent rate limited but you can turn this on. When its turned on and the connection rate exceeds the rate limit it can still send the status of the to the player when cache status is turned on. 
Disabling rate limiting can be done by setting it to 0 and allows as many connections as it can to be created. (There is no difference in rate limiting disconnect and offline disconnect packets yet.)

### state update Cooldown
To prevent a lot of calls being made to the backend without a reason Ultraviolet will keep track of the state from the backend. The state is currently being based on whether or not the backend will accept an tcp connection or not. When this happened and ultraviolet knows that the backend is `ONLINE` or `OFFLINE` it will wait the time the `stateUpdateCooldown` before it will set the state of the server back to `UNKNOWN`.  
Why is is doing this? This way Ultraviolet doesnt have to wait everytime someone is trying to connect to an offline server but only once every cooldown. This will speed up the process in general. 

## Config
- Time config values are based on go's duration formatting, valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". They can be used in combination with each other "1m30s".
- All config values left blank will result into their default value. For example if you dont have `"rateLimit": 5` inside your json, it will automatically put it on 0 which will also disable rate limit.  
- Inside the `examples` folder there is example of a server config file and the ultraviolet config file. 
- If its a place where you can use an ipv4, ipv6 should also work as well. Not specifying an ip and only using `:25565` will/might end up using either or both. 

### Ultraviolet Config
|Field name|Default | Description| 
|:---:|:---:|:---|
|listenTo|-|The address Ultraviolet will listen to for receiving connections.|
|defaultStatus|[this](#status-config-value)|The status Ultraviolet will send to callers when it receives a status handshake where the server address header isnt recognized.|
|numberOfWorkers|0|The number of name resolve workers Ultraviolet will have running, 0 will disabled it and makes is so that the proxy will receive and accept connections but it wont proxy or respond to them. 1 Should be able to handle quite a few connections. |


### Server Config

|Field name|Default | Description| 
|:---:|:---:|:---|
|domains|[""]|Place in here all urls which should be used by clients to target the backend.|
|proxyTo|-|It will call this ip/url when its creating a connection to the server.|
|proxyBind|-|The ip it should be using while connection to the backend. If it cant use the given value it will fail and the connection wont be created.|
|dialTimeout|1s|Timeout is the maximum amount of time a dial will wait for a connect to complete.|
|useRealIPv2.4|false|RealIP will only be used when players want to login. If both are turned on, it will use v2.4.|
|useRealIPv2.5|false|RealIP will only be used when players want to login. If both are turned on, it will use v2.4. If there isnt a key in the path, it will generate a key for you, the file of the key will begin with the first domain of this backend config.|
|realIPKeyPath|-|The path of the private key which will be used to encrypt the signature. Its not checking for file permissions or anything like that.|
|sendProxyProtocol|false|Whether or not it should send a ProxyProtocolv2 header to the target.|
|disconnectMessage|-|The message a user will get when its tries to connect to a offline server|
|offlineStatus|[this](#status-config-value)|The status it will send the player when the server is offline.|
|rateLimit|0|The number of connections it will allow to be made to the backend in the given `rateCooldown` time. 0 will disable rate limiting.|
|rateLimitStatus|false|Turning this on will result into status requests also counting towards created connections to the backend.| 
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
Like how [prometheus does it](https://github.com/prometheus/client_golang/blob/master/prometheus/gauge.go#L82) but than for things like state or rate limit so we can have more workers working on it and increase the load capacity even more. 

### Single liners
- respond in status with the same version the client has sent. 
- implement prometheus
- maybe anti bot stuff
