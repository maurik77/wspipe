# wspipe

``` cmd
# Webs Socket Server, exposes itself on http://localhost:8082
# accepts websocket connection at path /ws
# accepts http request at  path /route
# routes the http request to the path http://localhost:8090
go build . && .\connection-manager.exe -r server -p 8082 -s http://localhost:8090

# Webs Socket Client, exposes itself on http://localhost:8082
# connects via web socket to the server
# accepts http request at  path /route
# routes the http request to the path http://localhost:8091
go build . && .\connection-manager.exe -r client -w ws://localhost:8082/ws -p 8083 -s http://localhost:8091
```

``` cmd
cd .\utility\echoServer\
# The echo server accepts all requests
# if the request contains a body, it replies with the same body and http status 200
# otherwise, it replies with the "Hello!" message in the body and http status 200
go build .\server.go && .\server.exe -p 8090
go build .\server.go && .\server.exe -p 8091
```
