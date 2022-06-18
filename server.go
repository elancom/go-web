package web

import (
	"encoding/base64"
	"github.com/elancom/go-util/bytes"
	"github.com/elancom/go-util/crypto"
	"github.com/elancom/go-util/lang"
	"github.com/elancom/go-util/sign"
	"github.com/elancom/go-util/str"
	"github.com/gofiber/fiber/v2"
	"log"
	"net/http"
)

const loginUrl = "/login/"

var defConfig = Config{
	SignEnable: true,
	AuthEnable: true,
	EncEnable:  true,
}

func NewText(text string) *Text {
	t := new(Text)
	t.Text = text
	return t
}

type Text struct {
	Text string
}

func (t *Text) Error() string {
	return t.Text
}

func NewServer(config ...Config) *Server {
	s := new(Server)
	s.humanUrls = make([]string, 0)

	var conf = defConfig
	if len(config) > 0 {
		conf = config[0]
	}
	s.config = Config{
		SignEnable: conf.SignEnable,
		AuthEnable: conf.AuthEnable,
		EncEnable:  conf.EncEnable,
	}
	return s
}

type Config struct {
	AuthEnable bool // TK认证
	SignEnable bool // 签名认证(依赖TK认证)
	EncEnable  bool // 加密
}

type Server struct {
	App       *fiber.App
	config    Config
	humanUrls []string // prefix
}

func (s *Server) SetHumanUrls(urls []string) {
	humanUrls := make([]string, len(urls))
	for _, url := range urls {
		humanUrls = append(humanUrls, url)
	}
	s.humanUrls = humanUrls
}

func (s *Server) Init() *Server {
	s.App = s.newFiber()

	// 消息处理
	s.App.Use(func(c *fiber.Ctx) error {
		err := c.Next()

		if _, ok := err.(*lang.Msg); ok {
			log.Println("[返回JSON消息]", err)
			return c.JSON(err)
		}
		if err == lang.NotFound {
			log.Println("[返回JSON消息]", err.Error())
			return c.JSON(lang.NewErr(err.Error()))
		}
		if _, ok := err.(*Text); ok {
			log.Println("[返回文本消息]", err.Error())
			c.Response().Header.SetContentType(fiber.MIMETextPlain)
			return c.SendString(err.Error())
		}

		return err
	})

	// 认证
	s.App.Use(func(c *fiber.Ctx) error {
		if !s.config.AuthEnable {
			return c.Next()
		}

		path := c.Path()
		if len(path) > 0 {
			if str.HasPrefix(path, loginUrl) {
				return c.Next()
			}
			if len(s.humanUrls) > 0 {
				for _, url := range s.humanUrls {
					if str.HasPrefix(url, path) {
						return c.Next()
					}
				}
			}
		}

		principal, err := parseUserPrincipal(c)
		if err != nil {
			return err
		}

		// 设置认证信息到上下文
		c.Context().SetUserValue("principal", principal)

		return c.Next()
	})

	// 签名验证
	s.App.Use(func(c *fiber.Ctx) error {
		if !s.config.AuthEnable || !s.config.SignEnable {
			return c.Next()
		}

		principal, ok := c.Context().Value("principal").(*UserPrincipal)
		if !ok {
			return c.Next()
		}
		if principal.Secret == "" {
			return lang.NewErr("use x-enc, but secret not found")
		}

		xSign := c.Get("x-sign")
		log.Println("x-sign", xSign)
		if str.IsBlank(xSign) {
			return lang.NewErr("x-sign err")
		}

		switch c.Method() {
		case http.MethodGet:
			qs := c.Request().URI().QueryString()
			if len(qs) == 0 {
				return lang.NewErr("qs err")
			}
			if !sign.CheckStr(string(qs), principal.Secret, xSign) {
				return lang.NewErr("sign err")
			}
		case http.MethodPost:
			body := c.Body()
			if len(body) > 0 {
				body = bytes.TrimUint8(body, 34) // 34:双引号
			}
			if len(body) == 0 {
				return lang.NewErr("body err")
			}
			if !sign.CheckStr(string(body), principal.Secret, xSign) {
				return lang.NewErr("sign err")
			}
		}

		return c.Next()
	})

	// 解密
	s.App.Use(func(c *fiber.Ctx) error {
		if !s.config.EncEnable {
			return c.Next()
		}

		xEnc := c.Get("x-enc")
		if xEnc != "1" {
			return c.Next()
		}

		// 从tk中取加密秘钥
		principal, ok := c.Context().Value("principal").(*UserPrincipal)
		if !ok {
			return lang.NewErr("use x-enc, but not found principal")
		}
		if principal.Secret == "" {
			return lang.NewErr("use x-enc, but secret not found")
		}

		// 解密
		switch c.Method() {
		case http.MethodGet: // ?*****
			d3 := string(c.Request().URI().QueryString())
			log.Println("密文:", d3)
			if d3 != "" {
				d3b, err := base64.StdEncoding.DecodeString(d3)
				if err != nil {
					return err
				}
				decrypt, err := crypto.AesEcbDecrypt(d3b, []byte(principal.Secret))
				if err != nil {
					return lang.NewErr("dec err")
				}
				log.Println("解密:", string(decrypt))
				c.Request().URI().SetQueryStringBytes(decrypt)
			}
		case http.MethodPost:
			body := c.Body()
			if len(body) > 0 {
				body = bytes.TrimUint8(body, 34) // 34:双引号
				log.Println("密文:", string(body))
				bbs, err := base64.StdEncoding.DecodeString(string(body))
				if err != nil {
					return err
				}
				decrypt, err := crypto.AesEcbDecrypt(bbs, []byte(principal.Secret))
				if err != nil {
					return lang.NewErr("dec err")
				}
				log.Println("解密:", string(decrypt))
				// 修改内容及长度
				c.Request().SetBody(decrypt)
				c.Request().Header.Set("Content-Length", str.String(len(body)))
			}
		}
		return c.Next()
	})

	return s
}

// Listen Init app.Listen(":8080") app.Listen("127.0.0.1:8080")
func (s *Server) Listen(addr string) error {
	return s.App.Listen(addr)
}

func (s *Server) newFiber() *fiber.App {
	config := fiber.Config{
		// 禁止内部异常发送至外部
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.Println("[系统错误]", err)
			return c.JSON(lang.NewErr("InternalServerError"))
		}}
	fa := fiber.New(config)
	return fa
}
