# Ultraviolet
## What is it
[Infrared](https://github.com/haveachin/infrared) but different. Not even sure or this will be a real production ready product some day, its a different structure I want to try.  


## Some notes
### Im not gonna fool proof this
Im not planning on writing code to prevent Ultraviolet from crashing if you did something stupid. If the config wants a timeout time and you put in a negative number that may or may not cause some issues and that is your own fault. 

### Tableflip
This has implemented [tableflip](https://github.com/cloudflare/tableflip) which should make it able to reload/restart Ultraviolet without closing existing connections on Linux and macOS. Ultraviolet should still be usable on windows (testing purposes only pls). 
Check their [documentation](https://pkg.go.dev/github.com/cloudflare/tableflip) to know what or how. 


## Command-Line Flags

`-pid-file` specifies the path of the pid file ultraviolet will use [default: `"/run/ultraviolet.pid"`]

`-config` specifies the path to the main config [default: `"/etc/ultraviolet/ultraviolet.json"`]

`-server-configs` specifies the path to all your server configs [default: `"/etc/ultraviolet/config/"`]

## How does some stuff work

### state update Cooldown
To prevent a lot of calls being made to the backend without a reason Ultraviolet will keep track of the state from the backend. The state is currently being based on whether or not the backend will accept an tcp connection or not. When this happened and ultraviolet knows that the backend is `ONLINE` or `OFFLINE` it will wait the time the `stateUpdateCooldown` before it will set the state of the server back to `UNKNOWN`. 

## Config

time config values are based on go's duration formatting. They can be used in combination with each other "1m30s". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". 


### Server Config
- All config values left blank will result into their default value. For example if you dont have `"rateLimit": 5` inside your json, it will automatically put it on 0 which will also disable rate limit.  
- Inside the examples folder there is example of a server config file and the ultraviolet config file. 
- If its a place where you can use an ipv4, ipv6 should also work as well.

|Field name|Default | Description| 
|:---:|:---:|:---|
|domains|\[""\]|Place in here all urls which should be used by clients to target the backend.|
|proxyTo|""|It will call this ip/url when its creating a connection to the server.|
|proxyBind|""|The ip it should be using while connection to the server. Notice if it doesnt have access to that ip, it cant be assign and the connection to the backend will fail.|
|dialTimeout|1s|Timeout is the maximum amount of time a dial will wait for a connect to complete.|
|sendProxyProtocol|false|Whether or not it should send a ProxyProtocolv2 header to the target.|
|disconnectMessage|""|The message a user will get when its tries to connect to a offline server|
|offlineStatus|-|The status it will send the player when the server is offline.|
|rateLimit|0|The number of connections it will make to the backend. This includes connection used by Ultraviolet internally. It will reset when the `rateCooldown` time has passed. When the number has been exceeded but the cooldown isnt over yet Ultraviolet will behave like the server is offline. (this will stay like this until (online) status caching is possible.) |
|rateCooldown|1s|rateCooldown is the time which it will take before the rateLimit will be reset.|
|stateUpdateCooldown|1s|The time it will assume that the state of the server isnt changed (that server isnt offline now while it was online the last time we checked). |


