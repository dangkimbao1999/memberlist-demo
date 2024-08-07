// // package main

// // import (
// // 	"flag"
// // 	"fmt"
// // 	"log"
// // 	"os"
// // 	"os/signal"
// // 	"syscall"

// // 	"github.com/hashicorp/memberlist"
// // )

// // func main() {
// // 	// Define the port flag
// // 	port := flag.Int("port", 7946, "Port to bind for this node")
// // 	flag.Parse()

// // 	// Create a memberlist configuration
// // 	config := memberlist.DefaultLocalConfig()
// // 	config.Name = fmt.Sprintf("node-%d", *port)
// // 	config.BindPort = *port

// // 	// Create a new memberlist
// // 	list, err := memberlist.Create(config)
// // 	if err != nil {
// // 		log.Fatalf("Failed to create memberlist: %v", err)
// // 	}

// // 	// Join an existing cluster if addresses are provided
// // 	if len(os.Args) > 2 {
// // 		_, err := list.Join(os.Args[2:])
// // 		if err != nil {
// // 			log.Fatalf("Failed to join cluster: %v", err)
// // 		}
// // 	}

// // 	// Print the node's name
// // 	log.Printf("Node %s is running at %s:%d\n", config.Name, config.BindAddr, config.BindPort)

// // 	// Capture signals to handle graceful shutdown
// // 	sigCh := make(chan os.Signal, 1)
// // 	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
// // 	<-sigCh

// //		log.Println("Shutting down node")
// //		if err := list.Leave(0); err != nil {
// //			log.Fatalf("Failed to leave cluster: %v", err)
// //		}
// //	}
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/memberlist"
)

// CustomDelegate implements the memberlist.Delegate interface
type CustomDelegate struct {
	nodeName string
	queue    *memberlist.TransmitLimitedQueue
}

func (d *CustomDelegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *CustomDelegate) NotifyMsg(msg []byte) {
	log.Printf("[%s] Received message: %s\n", d.nodeName, string(msg))
}

func (d *CustomDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	broadcasts := d.queue.GetBroadcasts(overhead, limit)
	log.Printf("[%s] Broadcasting %d messages\n", d.nodeName, len(broadcasts))
	return broadcasts
}

func (d *CustomDelegate) LocalState(join bool) []byte {
	return []byte{}
}

func (d *CustomDelegate) MergeRemoteState(buf []byte, join bool) {}

// CustomBroadcast implements the memberlist.Broadcast interface
type CustomBroadcast struct {
	msg []byte
}

func (b *CustomBroadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

func (b *CustomBroadcast) Message() []byte {
	return b.msg
}

func (b *CustomBroadcast) Finished() {}

func main() {
	port := flag.Int("port", 7946, "Port to bind for this node")
	message := flag.String("message", "", "Message to send to the cluster")
	flag.Parse()

	config := memberlist.DefaultLocalConfig()
	config.Name = fmt.Sprintf("node-%d", *port)
	config.BindPort = *port

	delegate := &CustomDelegate{nodeName: config.Name}
	config.Delegate = delegate

	list, err := memberlist.Create(config)
	if err != nil {
		log.Fatalf("Failed to create memberlist: %v", err)
	}

	delegate.queue = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			return list.NumMembers()
		},
		RetransmitMult: 3,
	}

	if len(flag.Args()) > 0 {
		_, err := list.Join(flag.Args())
		if err != nil {
			log.Fatalf("Failed to join cluster: %v", err)
		}
	}

	log.Printf("Node %s is running at %s:%d\n", config.Name, config.BindAddr, config.BindPort)

	if *message != "" {
		log.Printf("Sending message: %s\n", *message)
		broadcast := &CustomBroadcast{
			msg: []byte(*message),
		}
		delegate.queue.QueueBroadcast(broadcast)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down node")
	if err := list.Leave(0); err != nil {
		log.Fatalf("Failed to leave cluster: %v", err)
	}
}
