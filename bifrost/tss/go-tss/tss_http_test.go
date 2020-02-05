package tss

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/libp2p/go-libp2p-core/protocol"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/tss/go-tss/common"
	"gitlab.com/thorchain/tss/go-tss/tss"
)

const testPriKey = "OTI4OTdkYzFjMWFhMjU3MDNiMTE4MDM1OTQyY2Y3MDVkOWFhOGIzN2JlOGIwOWIwMTZjYTkxZjNjOTBhYjhlYQ=="

func TestPackage(t *testing.T) { TestingT(t) }

type TssTestSuite struct{}

var _ = Suite(&TssTestSuite{})

func (t *TssTestSuite) SetUpSuite(c *C) {
	common.InitLog("info", true, "tss_http_test")
}

func setupTssForTest(c *C) *tss.TssServer {
	protocolID := protocol.ConvertFromStrings([]string{"tss"})[0]
	conf := common.TssConfig{}
	tss, err := tss.NewTss(nil, 6668, ":8080", ":8081", protocolID, []byte(testPriKey), "Asgard", "", true, conf)
	c.Assert(err, IsNil)
	c.Assert(tss, NotNil)
	return tss
}

func (t *TssTestSuite) TestHttpTssReusePort(c *C) {
	protocolID := protocol.ConvertFromStrings([]string{"tss"})[0]
	conf := common.TssConfig{}
	tss1, err := tss.NewTss(nil, 6660, "127.0.0.1:8080", ":8081", protocolID, []byte(testPriKey), "Asgard", "", true, conf)
	c.Assert(err, IsNil)
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

	tss2, err := tss.NewTss(nil, 6661, "127.0.0.1:8080", ":8082", protocolID, []byte(testPriKey), "Asgard", "", true, conf)
	ctx2, cancel2 := context.WithCancel(context.Background())
	err = tss2.Start(ctx2)
	c.Assert(err, ErrorMatches, "listen tcp 127.0.0.1:8080: bind: address already in use")
	cancel2()
	cancel()
	wg.Wait()
}

func (t *TssTestSuite) TestHttpNewTss(c *C) {
	protocolID := protocol.ConvertFromStrings([]string{"tss"})[0]
	conf := common.TssConfig{}
	tss, err := tss.NewTss(nil, 6668, ":12345", ":8081", protocolID, []byte(testPriKey), "Asgard", "", true, conf)
	c.Assert(err, IsNil)
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
	tssService.KeySign(respRecorder, req)
	c.Assert(respRecorder.Code, Equals, http.StatusMethodNotAllowed)

	respRecorder = httptest.NewRecorder()
	reqInvalidBody := httptest.NewRequest(http.MethodPost, "/keysign", bytes.NewBuffer([]byte("whatever")))
	tssService.KeySign(respRecorder, reqInvalidBody)
	c.Assert(respRecorder.Code, Equals, http.StatusBadRequest)
}
