pitaya-cli
==================

A REPL cli client made in go for pitaya.

## Installing

```
go install github.com/topfreegames/pitaya/pitaya-cli/v2@latest
```

## Usage

For cli flags, run `pitaya-cli --help`

```
$ pitaya-cli

Pitaya REPL Client
>>> help

Commands:
  clear             clear the screen
  connect           connects to pitaya
  disconnect        disconnects from pitaya server
  exit              exit the program
  help              display help
  notify            makes a notify to pitaya server
  push              insert information of push return
  request           makes a request to pitaya server
  sethandshake      sets a handshake parameter
```

### Protobuf
For connecting to a server that uses protobuf as serializer, the CLI needs to
know the protobuf descriptors to encode/decode the messages. You can find a
complete example in the [cluster_protobuf example](/examples/demo/cluster_protobuf).

When initializing the CLI, you have to provide the docs route as the following:
```
pitaya-cli -docs connector.docsHandler.docs
```

### Set handshake parameters

You can edit handshake parameters before connecting to the server.

You may pass the full handshake json:
```
Pitaya REPL Client
>>> sethandshake {"sys":{"clientVersion":"1.0.6", "clientBuildNumber":"999","platform":"ios"}}
```

Or edit one of three specific parameters:
```
Pitaya REPL Client
>>> sethandshake platform ios
>>> sethandshake buildNumber 999
>>> sethandshake version 1.0.6
```

### Read commands from file

It's possible to add a list of sequential requests into a file and pitaya-cli will execute them in order.

For example: commands.txt

```
connect localhost:3250
request connector.playerHandler.create
request connector.playerHandler.findmatch {"RoomType":"xxxx"}
```

Then run: `pitaya-cli --filename commands.txt`
