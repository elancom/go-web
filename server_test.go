package web

import (
	"encoding/base64"
	"github.com/elancom/go-util/crypto"
	"github.com/elancom/go-util/lang"
	"github.com/gofiber/fiber/v2"
	"io"
	"log"
	"net/http"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	server := NewServer(Config{
		SignEnable: false,
		AuthEnable: true,
		EncEnable:  true,
	})
	server.Init()

	server.App.Get("/get", func(c *fiber.Ctx) error {
		return lang.NewOk(time.Now())
	})

	request, err := http.NewRequest(http.MethodGet, "/get", nil)
	if err != nil {
		t.Error(err)
	}
	request.Header.Add("x-token", testToken)
	resp, err := server.App.Test(request)
	s, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	decodeString, err := base64.StdEncoding.DecodeString(string(s))
	if err != nil {
		t.Fatal(err.Error())
	}
	decrypt, err := crypto.AesEcbDecrypt(decodeString, []byte(testSecret))
	if err != nil {
		t.Fatal(err.Error())
	}
	log.Println("->", string(decrypt))
}
