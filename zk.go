package main

import (
	"fmt"
	"log"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// Circuit definition
type AdditionCircuit struct {
	A, B, C frontend.Variable
}

// Define declares the circuit's constraints
func (circuit *AdditionCircuit) Define(api frontend.API) error {
	// Enforce that A + B == C
	sum := api.Add(circuit.A, circuit.B)
	api.AssertIsEqual(sum, circuit.C)
	return nil
}

func main2() {
	// Define the circuit
	circuit := AdditionCircuit{}
	r1cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		log.Fatalf("Failed to compile circuit: %v", err)
	}

	// Generate proving and verifying keys
	pk, vk, err := groth16.Setup(r1cs)
	if err != nil {
		log.Fatalf("Failed to setup zk-SNARK: %v", err)
	}

	// Create a valid witness (A + B = C)
	assignment := AdditionCircuit{
		A: 3,
		B: 11,
		C: 14,
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

	// Create an invalid witness (A + B != C)
	invalidAssignment := AdditionCircuit{
		A: 3,
		B: 11,
		C: 16,
	}
	invalidWitness, err := frontend.NewWitness(&invalidAssignment, ecc.BN254.ScalarField())
	if err != nil {
		log.Fatalf("Failed to create invalid witness: %v", err)
	}

	// Generate the proof with invalid witness
	invalidProof, err := groth16.Prove(r1cs, pk, invalidWitness)
	if err != nil {
		log.Fatalf("Failed to create proof: %v", err)
	}

	// Extract public witness
	invalidPublicWitness, err := invalidWitness.Public()
	if err != nil {
		log.Fatalf("Failed to create public witness: %v", err)
	}

	// Verify the proof with invalid witness
	err = groth16.Verify(invalidProof, vk, invalidPublicWitness)
	if err != nil {
		fmt.Println("Proof is invalid, as expected!")
	} else {
		fmt.Println("Proof is valid! (unexpected)")
	}
}
