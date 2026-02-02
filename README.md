
# zprotocol 
zprotocol is an experimental, TCP-based application protocol built as part of the **YetiZ** project which is an alternative, JavaScript-free browser and web model.

Instead of extending HTTP, this project explores what a **minimal, purpose-built protocol** for static content delivery and site hosting could look like when: 
- the client and server are tightly controlled 
- the content model is static-first 
-  the protocol surface area is intentionally small

This repository contains the **reference implementation** of the protocol in Go: 
- a TCP server (`zserver`) 
- a client tool (`zclient`) 
- a host bridge used for domain-to-content routing (`zhost-bridge`)

This is an **experimental system**, designed for learning and exploration rather than production use.

## High-level idea 
At its core, zprotocol is about **direct communication**.

A client connects to a server over raw TCP and exchanges **explicit protocol commands** to: 
- request content 
-  validate domains / paths 
-  publish static assets (controlled and rule-based)

There are no HTTP headers, no middleware layers, and no browser scripting assumptions.  
Everything is explicit and handled at the application layer.

## Repository structure
```
zprotocol/  
├── client/  
│ └── zclient.go # TCP client for interacting with a zprotocol server  
├── server/  
│ └── zserver.go # Core TCP server implementing the protocol  
├── zhost-bridge/  
│ └── main.go # Domain and hosting bridge layer  
├── public/ # Sample/static content served by the system  
├── go.mod  
└── go.sum
 ```
 ## Components explained  
 ### 1. zserver ```(server/zserver.go)```  
 
 The  **zserver**  is  the  core  of  the  system.  It:  
 -  listens  on  a  TCP  port  
 -  accepts  incoming  client  connections  
 - reads  protocol  commands  line-by-line  
 -   parses  and  validates  requests  
 -   responds  with  structured  output  and  content  
 
 Key responsibilities:  
 -  handling  client  connections  
 - routing  requests  based  on  protocol  commands  
 -  enforcing  basic  validation  rules  
 - serving  files  from  mapped  directories  
 - managing  publish  and  retrieval  flows  

The  server  keeps  parsing  and  execution  logic  **intentionally  simple**  so  that  the  protocol  behavior  is  easy  to  follow  and  reason  about. 


---
 ### 2. zclient ```(client/zclient.go)```  
 The  **zclient**  is  a  command-line  client  used  to  interact  with  a  zprotocol  server. 
 It:  
 -  establishes  a  raw  TCP  connection  to  the  server  
 - sends  protocol  commands  
 - prints  server  responses  to  stdout  
 - -acts  as  both  a  testing  tool  and  a  functional  client  

This client is useful for:  
-  verifying  server  behavior  
- testing  protocol  flows 
- simulating  how  YetiZ  or  other  tools  would  interact  with  the  server 

The  client  does  not  abstract  away  protocol  details  it  exposes  them,  intentionally.

---
### 3. zhost-bridge ```(zhost-bridge/main.go)```  
The **zhost-bridge** acts as a glue layer between:  
-  protocol-level  domain  handling  
-  filesystem-backed  or  repository-backed  content  
	- It enables: 
		- mapping  domain  identifiers  (e.g.  `.z`  domains)  to  directories  
		- routing  requests  to  the  correct  content  root  
		- validating  domain  structure  and  access  rules  

This  component  exists  to  separate  **hosting  logic**  from  **core  protocol  logic**,  keeping  the  server  simpler  and  more  focused. 

## Protocol model (conceptual)  

zprotocol  operates  over  **plain  TCP**  and  follows  a  request–response  model.  
General flow:  
1.  Client  connects  to  the  server  
2. Client  sends  a  command  with  parameters
3. Server  validates  the  request  
4. Server responds with:  
	-  a  status  line  
	- optional  metadata  
	- optional  content  payload  
	
The protocol is:  
-  text-based  
- synchronous  
- easy  to  parse  
- intentionally  strict  Unlike  HTTP,  there is no attempt to support:  
	 -  streaming  media  
	 -  cookies  or  sessions  
	 - content  negotiation  
	 - client-side  execution  

## Example usage (development)  

### Run the server  
```
cd  server  
go  run  zserver.go
``` 

### Run the client
```
cd client
go run zclient.go 
```
Depending on the server configuration, the client can be used to:

-   request content
    
-   test domain routing
    
-   verify publish flows
    


## Design goals

This project was built to explore:

-   **Alternative web architecture**  
    What happens when we remove JavaScript, HTTP headers, and browser complexity?
    
-   **Explicit protocols**  
    Every action is a command. Nothing is implicit.
    
-   **Static-first content**  
    Predictable assets, predictable behavior.
    
-   **Small attack surface**  
    Fewer features means fewer assumptions.
    
-   **Educational clarity**  
    The code is meant to be read and understood.
    


## What this project is (and is not)

**This project is:**

-   an experiment
    
-   a learning tool
    
-   a protocol exploration
    
-   a systems-design exercise
    

**This project is not:**

-   a production-ready web replacement
    
-   a secure hosting platform
    
-   an HTTP competitor intended for real-world deployment
    


## Relationship to YetiZ

zprotocol serves as the **networking backbone** for **YetiZ**, an experimental browser built with:

-   a native UI
    
-   custom parsers
    
-   a JavaScript-free content model
    

Together, they explore what a simplified, alternative “web” could look like when designed from first principles.
