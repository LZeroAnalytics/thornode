package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	bkg "github.com/binance-chain/tss-lib/ecdsa/keygen"
	"github.com/hashicorp/go-retryablehttp"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/tss/go-tss/common"
	"gitlab.com/thorchain/tss/go-tss/tss"
)

func TestPackage(t *testing.T) { TestingT(t) }

type TssTestSuite struct {
	preParams []*bkg.LocalPreParams
}

var _ = Suite(&TssTestSuite{})

func (t *TssTestSuite) SetUpSuite(c *C) {
	if testing.Short() {
		c.Skip("Skipping, unit tests only")
	}
	common.InitLog("info", true, "tss_http_test")
	t.preParams = tss.getPreparams(c)
}

func setupTssForTest(c *C) *tss.TssServer {
	conf := common.TssConfig{}
	preParams := tss.getPreparams(c)
	priKey, err := tss.GetPriKey(tss.testPriKey)
	c.Assert(err, IsNil)
	tss, err := tss.NewTss(nil, 6668, priKey, "Asgard", "", conf, preParams[0])
	c.Assert(err, IsNil)
	tss.ConfigureHttpServers(
		":8080",
		":8081",
	)
	c.Assert(tss, NotNil)
	return tss
}

func (t *TssTestSuite) TestHttpTssReusePort(c *C) {
	conf := common.TssConfig{}
	priKey, err := tss.GetPriKey(tss.testPriKey)
	c.Assert(err, IsNil)
	tss1, err := tss.NewTss(nil, 6660, priKey, "Asgard", "", conf, t.preParams[0])
	c.Assert(err, IsNil)
	tss1.ConfigureHttpServers(
		"127.0.0.1:8080",
		":8081",
	)
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		err = tss1.Start(ctx)
		c.Assert(err, IsNil)
	}()
	_, err = retryablehttp.Get("http://127.0.0.1:8081/ping")
	c.Assert(err, IsNil)

	tss2, err := tss.NewTss(nil, 6661, priKey, "Asgard", "", conf, t.preParams[1])
	c.Assert(err, IsNil)
	tss2.ConfigureHttpServers(
		"127.0.0.1:8080",
		":8082",
	)
	ctx2, cancel2 := context.WithCancel(context.Background())
	err = tss2.Start(ctx2)
	c.Assert(err, ErrorMatches, "listen tcp 127.0.0.1:8080: bind: address already in use")
	cancel2()
	cancel()
	wg.Wait()
}

func (t *TssTestSuite) TestHttpNewTss(c *C) {
	conf := common.TssConfig{}
	priKey, err := tss.GetPriKey(tss.testPriKey)
	c.Assert(err, IsNil)
	tss, err := tss.NewTss(nil, 6668, priKey, "Asgard", "", conf, t.preParams[0])
	c.Assert(err, IsNil)
	tss.ConfigureHttpServers(
		":12345",
		":8081",
	)
	c.Assert(tss, NotNil)
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = tss.Start(ctx)
		c.Assert(err, IsNil)
	}()
	resp, err := retryablehttp.Get("http://127.0.0.1:8081/ping")
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	resp, err = http.Get("http://127.0.0.1:8081/p2pid")
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	result, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(len(result) > 0, Equals, true)
	c.Logf("p2p peer id: %s", result)
	cancel()
	wg.Wait()
}

func (t *TssTestSuite) TestHttpKeySign(c *C) {
	tssService := setupTssForTest(c)
	c.Assert(tssService, NotNil)
	respRecorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/keysign", nil)
	tssService.keySignHandler(respRecorder, req)
	c.Assert(respRecorder.Code, Equals, http.StatusMethodNotAllowed)

	respRecorder = httptest.NewRecorder()
	reqInvalidBody := httptest.NewRequest(http.MethodPost, "/keysign", bytes.NewBuffer([]byte("whatever")))
	tssService.keySignHandler(respRecorder, reqInvalidBody)
	c.Assert(respRecorder.Code, Equals, http.StatusBadRequest)
}
