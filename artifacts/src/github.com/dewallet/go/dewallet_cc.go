package main

import (
	"encoding/json"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"crypto"
	"crypto/x509"
	"crypto/rsa"
	"crypto/sha256"
	"errors"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = shim.NewLogger("dewallet_chaincodes")

// DewalletChaincode is chaincode for dewallet operation
type DewalletChaincode struct {
}

// Identity saves the identity of user
// Data is an encrypted data of the user
// Data can only be decrypted by user private key
type Identity struct {
	Username   string `json:"username"`
	PublicKey  string `json:"publicKey"`
	EPublicKey string `json:"ePublicKey"`
	SPublicKey string `json:"sPublicKey"`
	Data       string `json:"data"`
	Verified   string `json:"verified"`
	Keys       []Key  `json:"keys"`
}

// Key save the association between allowed user's username
// and encrypted key that can be used to decrypt the user data
type Key struct {
	Owner string `json:"for"`
	Key   string `json:"key"`
}

func (t *DewalletChaincode) VerifySignature(args []string, publicKey string) error {
	m := []byte(args[0])
	s, err := hex.DecodeString(args[1])
	if err != nil {
		return errors.New(fmt.Sprintf("Error in decoding signature %s", err))
	}

	pkBytes, err := base64.StdEncoding.DecodeString(publicKey)
	pk, err := x509.ParsePKIXPublicKey(pkBytes)
	if err != nil {
		return errors.New(fmt.Sprintf("Error in parsing key %s %s", publicKey, err))
	}

	switch pk := pk.(type) {
		case *rsa.PublicKey:
			h := sha256.Sum256(m)
			err = rsa.VerifyPKCS1v15(pk, crypto.SHA256, h[:], s)
			if err != nil {
				return errors.New(fmt.Sprintf("Error in verifying signature %s", err))
			}

			return nil
		default:
			return errors.New(fmt.Sprintf("Key is not RSA"))
	}
	
}

// Init will initialize the chaincode
func (t *DewalletChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	logger.Info("Initialize Dewallet Chaincode")
	return shim.Success(nil)
}

// Invoke will run the approriate function based on argument
func (t *DewalletChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	logger.Info("Invoking Dewallet Chaincode")

	function, args := stub.GetFunctionAndParameters()

	if function == "Register" {
		// Deletes an entity from its state
		return t.Register(stub, args)
	}

	if function == "UpdateUserData" {
		return t.UpdateUserData(stub, args)
	}

	if function == "AddKey" {
		return t.AddKey(stub, args)
	}

	if function == "GetPublicKey" {
		// queries an entity state
		return t.GetPublicKey(stub, args)
	}

	if function == "GetUserData" {
		return t.GetUserData(stub, args)
	}

	logger.Errorf("Unknown action, check the first argument, must be one of 'Register', 'GetPublicKey'. But got: %v", args[0])
	return shim.Error(fmt.Sprintf("Unknown action, check the first argument, must be one of 'Register', 'GetPublicKey'. But got: %v", args[0]))
}

// Register will add the user identity into blockchain
func (t *DewalletChaincode) Register(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	logger.Info("Registering a member")

	var i Identity
	json.Unmarshal([]byte(args[0]), &i)

	i.Keys = []Key{}

	iBytes, _ := json.Marshal(i)
	err := stub.PutState(i.Username, iBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(iBytes)
}

type updateUserDataRequest struct {
	Username string `json:"username"`
	Data     string `json:"data"`
}

type updateUserDataResponse struct {
	Data string `json:"data"`
}

// UpdateUserData will query the blockchain
// and update the encrypted data
func (t *DewalletChaincode) UpdateUserData(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	logger.Info("Updating data of user")

	var r updateUserDataRequest
	json.Unmarshal([]byte(args[0]), &r)

	iBytes, err := stub.GetState(r.Username)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if iBytes == nil {
		return shim.Error("Username not found")
	}

	var i Identity
	json.Unmarshal([]byte(iBytes), &i)

	err = t.VerifySignature(args, i.SPublicKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("Can't verify signature %s", err))
	}

	i.Data = r.Data

	iBytes, _ = json.Marshal(i)
	err = stub.PutState(i.Username, iBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(iBytes)
}


type addKeyRequest struct {
	Username string `json:"username"`
	Owner    string `json:"owner"`
	Key      string `json:"key"`
}

type addKeyResponse struct {
	Owner string `json:"owner"`
	Key   string `json:"key"`
}

// AddKey will add symetric key to blockchain
func (t *DewalletChaincode) AddKey(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	logger.Info("Adding decryption key of user data")

	var r addKeyRequest
	json.Unmarshal([]byte(args[0]), &r)

	iBytes, err := stub.GetState(r.Username)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if iBytes == nil {
		return shim.Error("Username not found")
	}

	key := Key{
		Owner: r.Owner,
		Key:   r.Key,
	}

	var i Identity
	json.Unmarshal([]byte(iBytes), &i)

	err = t.VerifySignature(args, i.SPublicKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("Can't verify signature %s", err))
	}

	i.Keys = append(i.Keys, key)
	iBytes, _ = json.Marshal(i)

	err = stub.PutState(i.Username, iBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	res := addKeyResponse{
		Owner: r.Owner,
		Key:   r.Key,
	}

	resBytes, _ := json.Marshal(res)

	return shim.Success(resBytes)
}

type getPublicKeyRequest struct {
	Username string `json:"username"`
}

type getPublicKeyResponse struct {
	PublicKey  string `json:"publicKey"`
	EPublicKey string `json:"ePublicKey"`
}

// GetPublicKey will query the blockchain
// to get the public key of a username
func (t *DewalletChaincode) GetPublicKey(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	logger.Info("Querying a member public key")

	var req getPublicKeyRequest
	json.Unmarshal([]byte(args[0]), &req)

	iBytes, err := stub.GetState(req.Username)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if iBytes == nil {
		return shim.Error("Username not found")
	}

	var i Identity
	json.Unmarshal([]byte(iBytes), &i)

	res := getPublicKeyResponse{
		PublicKey:  i.PublicKey,
		EPublicKey: i.EPublicKey,
	}

	resBytes, _ := json.Marshal(res)

	return shim.Success(resBytes)
}

type getUserDataRequest struct {
	Username string `json:"username"`
	Owner    string `json:"owner"`
}

type getUserDataResponse struct {
	PublicKey  string `json:"publicKey"`
	EPublicKey string `json:"ePublicKey"`
	SPublicKey string `json:"sPublicKey"`
	Data string `json:"data"`
	Key  string `json:"key"`
}

// GetUserData will query the blockchain
// and return encrypted data of a user
func (t *DewalletChaincode) GetUserData(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	logger.Info("Querying a user data")

	var req getUserDataRequest
	json.Unmarshal([]byte(args[0]), &req)

	iBytes, err := stub.GetState(req.Username)
	if err != nil {
		return shim.Error("Failed to get state")
	}
	if iBytes == nil {
		return shim.Error("Username not found")
	}

	var i Identity
	json.Unmarshal([]byte(iBytes), &i)

	var keyResult string

	for _, key := range i.Keys {
		if key.Owner == req.Owner {
			keyResult = key.Key
		}
	}

	res := getUserDataResponse{
		PublicKey: i.PublicKey,
		EPublicKey: i.EPublicKey,
		SPublicKey: i.SPublicKey,
		Data: i.Data,
		Key:  keyResult,
	}

	resBytes, _ := json.Marshal(res)

	return shim.Success(resBytes)
}


func main() {
	err := shim.Start(new(DewalletChaincode))
	if err != nil {
		logger.Errorf("Error starting Dewallet chaincode: %s", err)
	}
}
