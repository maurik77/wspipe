# Struttura
Il progetto consta di 4 files: 
1. Il file "connection.go" contiene la logica per gestire le connessioni websocket singole
1. il file "manager.go" contiene la logica per gestire un insieme di connessioni websocket. 
1. Il file "envelope.go" contiene la logica per incapsulare e decodificare i messaggi scambiati tra le connessioni websocket. 
1. Il file "main.go" contiene la logica di avvio del programma e la configurazione delle dipendenze.

## Connection.go
Il codice implementa una connessione WebSocket. La connessione WebSocket consente la comunicazione bidirezionale in tempo reale tra il client e il server. In questa implementazione, la connessione WebSocket viene utilizzata per scambiare messaggi appunto tra il client e il server.

Il pacchetto websocket di Gorilla viene utilizzato per creare e gestire la connessione WebSocket. Il pacchetto uuid di Google viene utilizzato per generare identificatori univoci per i messaggi scambiati sulla connessione.

La struttura connectionInstance rappresenta un'istanza di una connessione WebSocket. Essa contiene l'oggetto di connessione websocket.Conn, l'URL di base del server HTTP a cui inviare richieste, una canale send su cui inviare messaggi al server e una mappa di canali di risposta responseChannels, dove ogni canale viene utilizzato per ricevere la risposta ad una richiesta specifica inviata al server.

La funzione newInstance viene utilizzata per creare una nuova istanza di una connessione WebSocket. La funzione start viene utilizzata per avviare i processi di lettura e scrittura sulla connessione.

Le funzioni sendResponseAsync e sendRequestAsync vengono utilizzate per inviare rispettivamente una risposta o una richiesta al server. Entrambe queste funzioni serializzano il messaggio in un array di byte e lo inviano sulla canale send. La funzione sendRequestAsync inoltre crea un nuovo canale di risposta e lo associa all'identificatore del messaggio nella mappa di canali di risposta.

La funzione sendRequest viene utilizzata per inviare una richiesta al server e aspettare una risposta. Essa utilizza la funzione sendRequestAsync per inviare la richiesta e poi attende la risposta sulla relativa canale di risposta per un periodo di tempo specificato. Se il periodo di tempo scade senza che sia stata ricevuta alcuna risposta, viene restituito un errore.

I processi di lettura e scrittura vengono eseguiti dalle funzioni readPump e writePump, rispettivamente. Il processo di lettura legge i messaggi inviati dal server dalla connessione WebSocket e li gestisce in base al loro tipo. Il processo di scrittura invia i messaggi dalla canale send al server sulla connessione WebSocket.

Entrambi i processi utilizzano i costanti writeWait, pongWait, pingPeriod e `max

## Manager.go
Il codice implementa un gestore di connessioni WebSocket.

La struttura Manager rappresenta un gestore di connessioni WebSocket. Essa contiene una mappa di connessioni, dove ogni chiave è l'identificatore della connessione e il valore è l'istanza di connectionInstance associata. La struttura Manager viene utilizzata per gestire un insieme di connessioni WebSocket.

La funzione New viene utilizzata per creare una nuova istanza del gestore di connessioni.

Le funzioni add, get e SendRequestAsync e SendRequest vengono utilizzate per gestire le connessioni nel gestore. La funzione add viene utilizzata per aggiungere una nuova connessione al gestore, la funzione get viene utilizzata per ottenere una connessione specifica dal gestore e le funzioni SendRequestAsync e SendRequest vengono utilizzate per inviare rispettivamente una richiesta asincrona o sincrona ad una connessione specifica o a qualsiasi connessione presente nel gestore.

La funzione AddConnectionToPool viene utilizzata per aggiungere una nuova connessione WebSocket al gestore e avviare i processi di lettura e scrittura per la connessione.

Le costanti ClientRole e ServerRole vengono utilizzate per specificare il ruolo del gestore di connessioni, ovvero se esso rappresenta un client o un server.

## Envelope.go
Il codice rappresenta un insieme di funzioni (e strutture) per gestire i messaggi WebSocket.

La struttura payload rappresenta un messaggio WebSocket e contiene il tipo di messaggio, l'identificatore del messaggio e il contenuto del messaggio.

Le funzioni createRequestMessage e createResponseMessage vengono utilizzate per creare rispettivamente una richiesta o una risposta a partire da un oggetto http.Request o http.Response.

La funzione marshal viene utilizzata per codificare il contenuto del messaggio in un array di byte.

La funzione unmarshalPayload viene utilizzata per decodificare il contenuto di un array di byte in un oggetto payload.

Le funzioni getResponse e getRequest vengono utilizzate per ottenere rispettivamente un oggetto http.Response o http.Request a partire dal contenuto del messaggio.

La funzione getRequestWithDestinationURL viene utilizzata per ottenere un oggetto http.Request a partire dal contenuto del messaggio e per impostare l'URL di destinazione della richiesta utilizzando un URL di base specificato come argomento.

Le costanti RequestMessage e ResponseMessage vengono utilizzate per specificare il tipo di messaggio, ovvero se esso rappresenta una richiesta o una risposta.

## Main.go
Il codice è una implementazione di una soluzione che permette di fare il routing di richieste HTTP attraverso una connessione WebSocket.

La funzione main è il punto di ingresso del programma e configura i flag del comando (destination, port, role, wsURL) per essere utilizzati come parametri d'ingresso del programma.
Il flag role può assumere due valori: client o server.
In base al valore di role, viene chiamata la funzione manageClient o manageServer.

La funzione manageClient si occupa di instaurare una connessione WebSocket con l'URL specificato nel flag wsURL e di aggiungere la connessione appena creata al connection manager.

La funzione manageServer si occupa di gestire le richieste di connessione WebSocket che arrivano al server. Per ogni richiesta, viene creata una nuova connessione WebSocket con l'upgrader fornito e la connessione viene aggiunta al connection manager.

La funzione routingHandler è il gestore per le richieste HTTP che arrivano al server. Essa prende in input il connection manager e ritorna una funzione che prende in input una http.ResponseWriter e una *http.Request e che gestisce la richiesta in base al valore del flag role.
Se role è impostato a server, la richiesta viene inviata al primo client connesso presente nel connection manager.

# wspipe
``` cmd
# Webs Socket Server, exposes itself on http://localhost:8082
# accepts websocket connection at path /ws
# accepts http request at  path /route
# routes the http request to the path http://localhost:8090
go build . && .\wspipe.exe -r server -p 8082 -s http://localhost:8090

# Webs Socket Client, exposes itself on http://localhost:8082
# connects via web socket to the server
# accepts http request at  path /route
# routes the http request to the path http://localhost:8091
go build . && .\wspipe.exe -r client -w ws://localhost:8082/ws -p 8083 -s http://localhost:8091
```

``` cmd
cd .\utility\echoServer\
# The echo server accepts all requests
# if the request contains a body, it replies with the same body and http status 200
# otherwise, it replies with the "Hello!" message in the body and http status 200
go build .\server.go && .\server.exe -p 8090
go build .\server.go && .\server.exe -p 8091
```

## CURL

``` cmd
 curl -X POST http://localhost:8083/route/test
 curl -X POST http://localhost:8082/route/7ff9763b-009a-466a-b864-31b01d27da6c/test -d "test"
```
