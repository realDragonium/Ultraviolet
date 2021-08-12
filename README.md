# Ultraviolet - Alpha v0.12.2

## What is Ultraviolet?
Its a reverse minecraft proxy, capable of serving as a placeholder when the server is offline for status response to clients.   

one could also say [infrared](https://github.com/haveachin/infrared) but different.

Everything which ultraviolet has or does right now is not final its possible that it will change how it currently works therefore the reason its still in Alpha. If wanna complain that something has been changed and/or it broke something thats your own fault, its in Alpha version after all. 

Thinks likely to change: 
- Run command
- config file(s) (structure of the files themselves)


## Features
[x] Proxy Protocol(v2) support  
[x] RealIP (v2.4&v2.5)  
[x] Rate limiting -> Login verification  
[x] Status caching (online status only)  
[x] Offline status placeholder  
[x] Prometheus Support  
[x] API (Reload server config files, more later)  
... More coming later?

## Some notes
### Limited connections when running binary
Because linux the default settings for fd is 1024, this means that you can by default Ultraviolet can have 1024 open connections before it starts refusing connections because it cant open anymore fds. Because of some internal queues you should consider increasing the limit if you expect to proxy over 900 open connections at the same time. 

### How to build
Ultraviolet can be ran by using docker or you can also build a binary yourself by running:
```
$ cd cmd/Ultraviolet/
$ go build
```  

### How to run

Ultraviolet will, when no config is specified by the command, use `/etc/ultraviolet` as work dir and create here an `ultraviolet.json` file for you.
```
$ ./ultraviolet run
```  

### Tableflip
This has implemented [tableflip](https://github.com/cloudflare/tableflip) which should make it able to reload/hotswap Ultraviolet without closing existing connections on Linux and macOS. Ultraviolet should still be usable on windows (testing purposes only pls). 
Check their [documentation](https://pkg.go.dev/github.com/cloudflare/tableflip) to know what or how. 

IMPORTANT: There is a limit of one 'parent' process. So when you reload Ultraviolet once you need to wait until the parent process is closed (all previous connections have been closed) before you can reload it again.

## Command-Line 
The follows commands can be used with ultraviolet, all flags (if related) should work for every command and be used by every command if you used it for one command.

So far it only can use:
- run
- reload

### Flags
`-configs` specifies the path to the config directory [default: `"/etc/ultraviolet/"`]  


# Config
Check the [wiki](https://github.com/realDragonium/Ultraviolet/wiki/Config) for more information about the config.  
