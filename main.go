package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/backkem/gowttest/buffer"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v2"
)

const (
	trackerURL = `wss://tracker.openwebtorrent.com/` // For simplicity
)

func main() {
	meta, err := metainfo.LoadFromFile("./sintel.torrent")
	if err != nil {
		log.Fatalf("failed to load meta info: %v\n", err)
	}

	info, err := meta.UnmarshalInfo()
	if err != nil {
		log.Fatalf("failed to unmarshal info: %v\n", err)
	}
	left := info.TotalLength()

	infoHash := meta.HashInfoBytes().String()
	b, err := buffer.FromHex(infoHash)
	if err != nil {
		log.Fatalf("failed to create buffer: %v\n", err)
	}
	infoHashBinary := b.ToStringLatin1()

	config := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}}
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Fatalf("failed to peer connection: %v\n", err)
	}
	dataChannel, err := peerConnection.CreateDataChannel("webrtc-datachannel", nil)
	if err != nil {
		log.Fatalf("failed to data channel: %v\n", err)
	}
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	dataChannel.OnOpen(func() {
		fmt.Printf("Data channel '%s'-'%d' open. Random messages will now be sent to any connected DataChannels every 5 seconds\n", dataChannel.Label(), dataChannel.ID())
		// TODO
	})
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		fmt.Printf("Message from DataChannel '%s': '%s'\n", dataChannel.Label(), string(msg.Data))
	})
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatalf("failed to create offer: %v\n", err)
	}
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		log.Fatalf("failed to set local description: %v\n", err)
	}

	randPeerID, err := buffer.RandomBytes(9)
	if err != nil {
		log.Fatalf("failed to generate bytes: %v\n", err)
	}
	peerIDBuffer := buffer.From("-WW0007-" + randPeerID.ToStringBase64())
	// peerID := peerIDBuffer.ToStringHex()
	peerIDBinary := peerIDBuffer.ToStringLatin1()

	randOfferID, err := buffer.RandomBytes(20)
	if err != nil {
		log.Fatalf("failed to generate bytes: %v\n", err)
	}
	// OfferID := randOfferID.ToStringHex()
	offerIDBinary := randOfferID.ToStringLatin1()

	req := AnnounceRequest{
		Numwant:    1,
		Uploaded:   0,
		Downloaded: 0,
		Left:       int(left),
		Event:      "started",
		Action:     "announce",
		InfoHash:   infoHashBinary,
		PeerID:     peerIDBinary,
		Offers: []Offer{
			{
				OfferID: offerIDBinary,
				Offer:   offer,
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("failed to marshal request: %v\n", err)
	}

	fmt.Println(string(data))

	c, _, err := websocket.DefaultDialer.Dial(trackerURL, nil)
	if err != nil {
		log.Fatal("failed to dial tracker:", err)
	}
	defer c.Close()

	go func() {
		data, err := json.Marshal(req)
		if err != nil {
			log.Fatal("failed to marshal request:", err)
		}
		err = c.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Fatal("write:", err)
		}
	}()

	for i := 0; i < 100; i++ {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Fatal("read:", err)
			return
		}
		var buf bytes.Buffer
		json.Indent(&buf, message, "  ", "  ")
		log.Printf("received message from tracker: %s", buf.String())

		var ar AnnounceResponse
		if err := json.Unmarshal(message, &ar); err != nil {
			log.Printf("error unmarshalling announce response: %v", err)
			continue
		}
		if ar.Answer == nil {
			continue
		}
		if err := peerConnection.SetRemoteDescription(*ar.Answer); err != nil {
			log.Printf("error setting remote description: %v", err)
			continue
		}
	}
}

type AnnounceRequest struct {
	Numwant    int     `json:"numwant"`
	Uploaded   int     `json:"uploaded"`
	Downloaded int     `json:"downloaded"`
	Left       int     `json:"left"`
	Event      string  `json:"event"`
	Action     string  `json:"action"`
	InfoHash   string  `json:"info_hash"`
	PeerID     string  `json:"peer_id"`
	Offers     []Offer `json:"offers"`
}

type Offer struct {
	OfferID string                    `json:"offer_id"`
	Offer   webrtc.SessionDescription `json:"offer"`
}

type AnnounceResponse struct {
	InfoHash   string                     `json:"info_hash"`
	Action     string                     `json:"action"`
	Interval   *int                       `json:"interval,omitempty"`
	Complete   *int                       `json:"complete,omitempty"`
	Incomplete *int                       `json:"incomplete,omitempty"`
	PeerID     string                     `json:"peer_id,omitempty"`
	Answer     *webrtc.SessionDescription `json:"answer,omitempty"`
	OfferID    string                     `json:"offer_id,omitempty"`
}
