#Go Stomp 1.2 Client
A STOMP 1.2 Client.

See https://stomp.github.io/stomp-specification-1.2.html for more information.

##Examples

####Send
```
package main

import (
	"log"
	"strings"
	"time"

	"github.com/djoyahoy/stomp"
)

func main() {
	c, err := stomp.Connect(":61613", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Disconnect()

	for i := 0; i < 10; i++ {
		log.Println("Send")
		err := c.Send("/queue/test", nil, "text/plain", strings.NewReader(time.Now().String()), false)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(time.Second * 2)
	}
}
```

####Subscribe
```
package main

import (
	"io/ioutil"
	"log"
	"time"

	"github.com/djoyahoy/stomp"
)

func main() {
	c, err := stomp.Connect(":61613", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Disconnect()

	id, err := c.Subscribe("/queue/test", stomp.ClientMode, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(id)

	for {
		select {
		case msg, ok := <-c.MsgCh:
			if !ok {
				return
			}

			buf, err := ioutil.ReadAll(msg.Body)
			if err != nil {
				c.Nack(msg.Headers["ack"], false)
				continue
			}

			log.Println(string(buf))

			err = c.Ack(msg.Headers["ack"], false)
			if err != nil {
				log.Fatal(err)
			}
		case err := <-c.ErrCh:
			log.Fatal(err)
		case <-time.After(time.Second * 60):
			return
		}
	}
}
```
