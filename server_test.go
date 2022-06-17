package web

import (
	"github.com/gofiber/fiber/v2"
	"io"
	"log"
	"net/http"
	"testing"
)

func TestName(t *testing.T) {
	server := NewServer(Config{
		SignEnable: false,
		AuthEnable: false,
		EncEnable:  false,
	})
	server.Init()

	server.App.Get("/get", func(c *fiber.Ctx) error {
		return c.JSON(map[string]any{
			"code": 200,
		})
	})

	stop := make(chan any)

	go func() {
		request, err := http.NewRequest(http.MethodGet, "/get", nil)
		if err != nil {
			t.Error(err)
		}
		resp, err := server.App.Test(request)
		s, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
		}
		log.Println(string(s))

		stop <- 1
	}()

	go func() {
		<-stop
		_ = server.App.Shutdown()
		log.Println("OK")
	}()

	err := server.Listen(":8888")
	if err != nil {
		t.Error(err)
	}

}
