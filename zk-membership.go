package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/hashicorp/memberlist"
)

// Circuit definition
type MultiplyCircuit struct {
	A, B, C frontend.Variable
}

// Define declares the circuit's constraints
func (circuit *MultiplyCircuit) Define(api frontend.API) error {
	// Enforce that A * B == C
	sum := api.Mul(circuit.A, circuit.B)
	api.AssertIsEqual(sum, circuit.C)
	return nil
}

// CustomDelegate implements the memberlist.Delegate interface
type CustomDelegate1 struct {
	nodeName string
	queue    *memberlist.TransmitLimitedQueue
	vk       groth16.VerifyingKey
}

func (d *CustomDelegate1) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *CustomDelegate1) NotifyMsg(msg []byte) {
	log.Printf("[%s] Received message: %s\n", d.nodeName, msg)

	var proof groth16.Proof = groth16.NewProof(ecc.BN254)

	// cs := groth16.NewCS(ecc.BN254)
	// _, err := proof.ReadFrom(bytes.NewBuffer(msg))
	// if err != nil {
	// 	log.Printf("[%s] Failed to unmarshal proof: %v\n", d.nodeName, err)
	// 	return
	// }
	circuit := MultiplyCircuit{}

	r1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		log.Fatalf("Failed to compile circuit: %v", err)
	}
	pk, vk, err := groth16.Setup(r1cs)
	if err != nil {
		log.Fatalf("Failed to setup zk-SNARK: %v", err)
	}
	if msg == nil {
		log.Printf("[%s] Received nil message\n", d.nodeName)
		return
	}

	buffer := bytes.NewBuffer(msg)
	if buffer == nil {
		log.Printf("[%s] Failed to create buffer from message\n", d.nodeName)
		return
	}

	// _, err := proof.ReadFrom(buffer)
	// if err != nil {
	// 	log.Printf("[%s] Failed to unmarshal proof: %v\n", d.nodeName, err)
	// 	return
	// }

	dec := gob.NewDecoder(buffer)
	err = dec.Decode(proof)
	if err != nil {
		log.Fatal("encode error:", err)
	}

	log.Printf("[%s] Proof unmarshalled: %v\n", d.nodeName, proof)

	assignment := MultiplyCircuit{
		A: 1,
		B: 1,
		C: 1,
	}

	witness, err := frontend.NewWitness(&assignment, ecc.BN254.ScalarField())
	if err != nil {
		log.Printf("[%s] Failed to create public witness: %v\n", d.nodeName, err)
		return
	}
	proof, err = groth16.Prove(r1cs, pk, witness)
	if err != nil {
		log.Fatalf("Failed to create proof: %v", err)
	}

	publicWitness, err := witness.Public()
	if err != nil {
		log.Fatalf("Failed to create public witness: %v", err)
	}

	if err := groth16.Verify(proof, vk, publicWitness); err != nil {
		log.Printf("[%s] Invalid proof: %v\n", d.nodeName, err)
	} else {
		log.Printf("[%s] Valid proof received.\n", d.nodeName)
	}
}

func (d *CustomDelegate1) GetBroadcasts(overhead, limit int) [][]byte {
	broadcasts := d.queue.GetBroadcasts(overhead, limit)
	log.Printf("[%s] Broadcasting %d messages\n", d.nodeName, len(broadcasts))
	return broadcasts
}

func (d *CustomDelegate1) LocalState(join bool) []byte {
	return []byte{}
}

func (d *CustomDelegate1) MergeRemoteState(buf []byte, join bool) {}

// CustomBroadcast implements the memberlist.Broadcast interface
type CustomBroadcast1 struct {
	msg []byte
}

func (b *CustomBroadcast1) Invalidates(other memberlist.Broadcast) bool {
	return false
}

func (b *CustomBroadcast1) Message() []byte {
	return b.msg
}

func (b *CustomBroadcast1) Finished() {}

func main() {
	port := flag.Int("port", 7946, "Port to bind for this node")
	message := flag.String("message", "", "Message to send to the cluster")
	flag.Parse()

	config := memberlist.DefaultLocalConfig()
	config.Name = fmt.Sprintf("node-%d", *port)
	config.BindPort = *port

	// Setup zk-SNARKs
	circuit := MultiplyCircuit{}
	r1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		log.Fatalf("Failed to compile circuit: %v", err)
	}

	pk, vk, err := groth16.Setup(r1cs)
	if err != nil {
		log.Fatalf("Failed to setup zk-SNARK: %v", err)
	}

	delegate := &CustomDelegate1{nodeName: config.Name, vk: vk}
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

		// Create a valid witness (A * B = C)
		assignment := MultiplyCircuit{
			A: 1,
			B: 1,
			C: 1,
		}
		witness, err := frontend.NewWitness(&assignment, ecc.BN254.ScalarField())
		if err != nil {
			log.Fatalf("Failed to create witness: %v", err)
		}

		// Generate the proof
		proof, err := groth16.Prove(r1cs, pk, witness)
		if err != nil {
			log.Fatalf("Failed to create proof: %v", err)
		}
		log.Println("Verifying internal...")
		// Extract public witness
		publicWitness, err := witness.Public()
		if err != nil {
			log.Fatalf("Failed to create public witness: %v", err)
		}

		// Verify the proof
		err = groth16.Verify(proof, vk, publicWitness)
		if err != nil {
			log.Fatalf("Failed to verify proof: %v", err)
		} else {
			fmt.Println("Proof is valid!")
		}
		// -------------------------------------------
		var buf bytes.Buffer

		enc := gob.NewEncoder(&buf)
		err = enc.Encode(proof)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		// _, err = proof.WriteTo(&buf)
		// if err != nil {
		// 	log.Fatalf("Failed to marshal proof: %v", err)
		// }
		// readBuf, _ := ioutil.ReadAll(&buf)
		broadcast := &CustomBroadcast1{
			msg: buf.Bytes(),
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
