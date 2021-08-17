# Ultraviolet

## What is Ultraviolet?
Its a reverse minecraft proxy, capable of serving as a placeholder when the server is offline for status response to clients. It also has some basic anti backend ddos features. So can it ask players to verify themselves when there are to many players trying to join within a given timeframe and it will (by default) cache the status of the server.  


## Extra Features
[x] Proxy Protocol(v2) support  
[x] RealIP (v2.4&v2.5)  
[x] Rate limiting -> Login verification  
[x] Status caching (online status only)  
[x] Offline status placeholder  
[x] Prometheus Support  
[x] API (Reload server config files, more later)  


## How to run
Ultraviolet will, when no config is specified by the command, use `/etc/ultraviolet` as work dir and create here an `ultraviolet.json` file for you.
```
$ ./ultraviolet run
```  

## How to build
Ultraviolet can be ran by using docker or you can also build a binary yourself by running:
```
$ cd cmd/Ultraviolet/
$ go build
```  

## How to run with docker
You can run ultraviolet in docker by executing the follow command:
```
docker run -d -p 25565:25565 -v /etc/ultraviolet:/etc/ultraviolet --restart=unless-stopped realdragonium/ultraviolet:latest
```
You could also pull it from github packages with
```
docker pull ghcr.io/realdragonium/ultraviolet:latest
``` 
Or you can make one yourself with the `Dockerfile` in the root folder of the project.

## Some notes
### Limited connections when running binary
Because linux the default settings for fd is 1024, this means that you can by default Ultraviolet can have 1024 open connections before it starts refusing connections because it cant open anymore fds. Because of some internal queues you should consider increasing the limit if you expect to proxy over 900 open connections at the same time. 

### File examples
In the folder `examples` in the project are a few related file examples which can be used together with Ultraviolet. There is also a basic grafana dashboard layout there which can be used together with the prometheus feature.

### Hotswapping to newer versions
Its not necessary to use this to reload the server configs, there is also an api or an command if you want to reload the server configs. 

This has implemented [tableflip](https://github.com/cloudflare/tableflip) which should make it able to reload/hotswap Ultraviolet without closing existing connections on Linux and macOS. Ultraviolet should still be usable on windows (testing purposes only pls). 
Check their [documentation](https://pkg.go.dev/github.com/cloudflare/tableflip) to know what or how. 

IMPORTANT (when using this feature): There is a limit of one 'parent' process. So when you reload Ultraviolet once you need to wait until the parent process is closed (all previous connections have been closed) before you can reload it again.

## Command-Line 
The follows commands can be used with ultraviolet, all flags (if related) should work for every command and be used by every command if you used it for one command.

So far it only can use:
- run
- reload

### Flags
`-config` specifies the path to the config directory [default: `"/etc/ultraviolet/"`]  (if you want to use this directory you dont have to use this flag.)


# Config
Check the [wiki](https://github.com/realDragonium/Ultraviolet/wiki/Config) for more information about the config.  

There are also actual file which can be used as examples in the example folder.