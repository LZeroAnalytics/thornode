package tss

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	bkeygen "github.com/binance-chain/tss-lib/ecdsa/keygen"
	"github.com/libp2p/go-libp2p-core/protocol"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	cryptokey "github.com/tendermint/tendermint/crypto"
	"golang.org/x/sync/errgroup"

	"gitlab.com/thorchain/tss/go-tss/common"
	"gitlab.com/thorchain/tss/go-tss/keygen"
	"gitlab.com/thorchain/tss/go-tss/keysign"
	"gitlab.com/thorchain/tss/go-tss/p2p"
)

type TssServer struct {
	conf             common.TssConfig
	logger           zerolog.Logger
	Status           common.TssStatus
	tssHttpServer    *http.Server
	infoHttpServer   *http.Server
	p2pCommunication *p2p.Communication
	priKey           cryptokey.PrivKey
	preParams        *bkeygen.LocalPreParams
	wg               sync.WaitGroup
	tssKeyGenLocker  *sync.Mutex
	tssKeySignLocker *sync.Mutex
	stopChan         chan struct{}
	subscribers      map[string]chan *p2p.Message
	homeBase         string
}

// NewTss create a new instance of Tss
func NewTss(
	bootstrapPeers []maddr.Multiaddr,
	p2pPort int,
	protocolID protocol.ID,
	priKeyBytes []byte,
	rendezvous,
	baseFolder string,
	conf common.TssConfig,
	preParams *bkeygen.LocalPreParams,
) (*TssServer, error) {
	priKey, err := getPriKey(string(priKeyBytes))
	if err != nil {
		return nil, errors.New("cannot parse the private key")
	}

	P2PServer, err := p2p.NewCommunication(rendezvous, bootstrapPeers, p2pPort, protocolID)
	if err != nil {
		return nil, fmt.Errorf("fail to create communication layer: %w", err)
	}

	// When using the keygen party it is recommended that you pre-compute the
	// "safe primes" and Paillier secret beforehand because this can take some
	// time.
	// This code will generate those parameters using a concurrency limit equal
	// to the number of available CPU cores.
	if preParams == nil || !preParams.Validate() {
		preParams, err = bkeygen.GeneratePreParams(conf.PreParamTimeout)
		if err != nil {
			return nil, fmt.Errorf("fail to generate pre parameters: %w", err)
		}
	}
	if !preParams.Validate() {
		return nil, errors.New("invalid preparams")
	}

	tssServer := TssServer{
		conf:   conf,
		logger: log.With().Str("module", "tss").Logger(),
		Status: common.TssStatus{
			Starttime: time.Now(),
		},
		p2pCommunication: P2PServer,
		priKey:           priKey,
		preParams:        preParams,
		tssKeyGenLocker:  &sync.Mutex{},
		tssKeySignLocker: &sync.Mutex{},
		stopChan:         make(chan struct{}),
		subscribers:      make(map[string]chan *p2p.Message),
		homeBase:         baseFolder,
	}

	return &tssServer, nil
}

func (t *TssServer) ConfigureHttpServers(tss, info string) {
	t.tssHttpServer = NewTssHttpServer(tss, t)
	t.infoHttpServer = NewInfoHttpServer(info, t)
}

func (t *TssServer) StartHttpServers() error {
	if t.tssHttpServer == nil || t.infoHttpServer == nil {
		return nil
	}

	defer t.wg.Done()
	ctx := context.Background()
	g, newCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		err := t.tssHttpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	g.Go(func() error {
		err := t.infoHttpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Failed to start info HTTP server")
			return err
		}
		return nil
	})
	g.Go(func() error {
		select {
		case <-t.stopChan:
		case <-newCtx.Done():
		}
		err := StopServer(t.tssHttpServer)
		err2 := StopServer(t.infoHttpServer)
		if err != nil || err2 != nil {
			log.Error().Err(err).Msg("Failed to shutdown the Tss or info server gracefully")
			return errors.New("error in shutdown gracefully")
		}
		return nil
	})
	return g.Wait()
}

// Start Tss server
func (t *TssServer) Start(ctx context.Context) error {
	log.Info().Msg("Starting the HTTP servers")
	t.Status.Starttime = time.Now()
	t.wg.Add(1)
	go func() {
		<-ctx.Done()
		close(t.stopChan)
		// stop the p2p and finish the p2p wait group
		err := t.p2pCommunication.Stop()
		if err != nil {
			t.logger.Error().Msgf("error in shutdown the p2p server")
		}
	}()

	prikeyBytes, err := getPriKeyRawBytes(t.priKey)
	if err != nil {
		return err
	}

	go t.p2pCommunication.ProcessBroadcast()
	if err := t.p2pCommunication.Start(prikeyBytes); nil != err {
		return fmt.Errorf("fail to start p2p communication layer: %w", err)
	}
	err = t.StartHttpServers()
	if err != nil {
		return err
	}
	t.wg.Wait()
	log.Info().Msg("The Tss and p2p server has been stopped successfully")
	return nil
}

func (t *TssServer) requestToMsgId(request interface{}) (string, error) {
	var dat []byte
	switch value := request.(type) {
	case keygen.KeyGenReq:
		keyAccumulation := ""
		keys := value.Keys
		sort.Strings(keys)
		for _, el := range keys {
			keyAccumulation += el
		}
		dat = []byte(keyAccumulation)
	case keysign.KeySignReq:
		msgToSign, err := base64.StdEncoding.DecodeString(value.Message)
		if err != nil {
			t.logger.Error().Err(err).Msg("error in decode the keysign req")
			return "", err
		}
		dat = msgToSign
	default:
		t.logger.Error().Msg("unknown request type")
		return "", errors.New("unknown request type")
	}

	msgID, err := common.MsgToHashString(dat)
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to hash the message")
		return "", err
	}
	return msgID, nil
}
