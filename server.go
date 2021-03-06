package web

import (
	"encoding/base64"
	"github.com/elancom/go-util/bytes"
	"github.com/elancom/go-util/crypto"
	"github.com/elancom/go-util/json"
	. "github.com/elancom/go-util/lang"
	"github.com/elancom/go-util/sign"
	"github.com/elancom/go-util/str"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"log"
	"net/http"
)

func newDefaultConfig() Config {
	return Config{
		SignEnable: true,
		AuthEnable: true,
		EncEnable:  true,

		// 地址过来
		IgnoreUrls: make([]string, 0),

		// 跨域配置
		CorsEnable: false,
	}
}

func NewText(text string) *Text {
	t := new(Text)
	t.text = text
	return t
}

type Text struct {
	text string
}

func (t *Text) Error() string {
	return t.text
}

func NewServer(config ...Config) *Server {
	s := new(Server)
	s.ignoreUrls = make([]string, 0)

	var conf Config
	if len(config) > 0 {
		conf = config[0]
	} else {
		conf = newDefaultConfig()
	}
	s.config = conf

	// 权限忽略地址
	s.setIgnoreUrls(s.config.IgnoreUrls)

	return s
}

type Config struct {
	AuthEnable bool     // TK认证
	SignEnable bool     // 签名认证(依赖TK认证)
	EncEnable  bool     // 加密
	IgnoreUrls []string // 忽略地址

	// 跨域配置
	CorsEnable       bool // 是否开启跨域
	AllowOrigins     string
	AllowMethods     string
	AllowHeaders     string
	AllowCredentials bool
	ExposeHeaders    string
	MaxAge           int
}

type Server struct {
	App        *fiber.App
	config     Config
	ignoreUrls []string // 如果很多再用map
}

func (s *Server) setIgnoreUrls(urls []string) {
	if len(urls) == 0 {
		return
	}
	ignoreUrls := make([]string, len(urls))
	for _, url := range urls {
		ignoreUrls = append(ignoreUrls, url)
	}
	s.ignoreUrls = ignoreUrls
}

func (s *Server) Init() *Server {
	s.App = s.newFiber()

	if s.config.CorsEnable {
		s.App.Use(cors.New(cors.Config{
			AllowOrigins:     s.config.AllowOrigins,
			AllowHeaders:     s.config.AllowHeaders,
			AllowMethods:     s.config.AllowMethods,
			AllowCredentials: s.config.AllowCredentials,
			ExposeHeaders:    s.config.ExposeHeaders,
			MaxAge:           s.config.MaxAge,
		}))
	}

	// 加密字符串
	encStr := func(principal *UserPrincipal, s string) (string, error) {
		sb, encErr := crypto.AesEcbEncrypt([]byte(s), []byte(principal.Secret))
		if encErr != nil {
			return "", NewErr("enc err")
		}
		return base64.StdEncoding.EncodeToString(sb), nil
	}

	// 消息处理
	s.App.Use(func(c *fiber.Ctx) error {
		log.Println("处理")
		err := c.Next()
		log.Println("处理after")

		if err == nil {
			err = NewErr("处理器响应空消息")
		}

		if _, ok := err.(*Msg); ok {
			js, _ := json.ToJson(err)
			log.Println("[返回JSON消息]", js)
			return c.JSON(err)
		}
		if _, ok := err.(*Text); ok {
			log.Println("[返回文本消息]", err.Error())
			c.Response().Header.SetContentType(fiber.MIMETextPlain)
			return c.SendString(err.Error())
		}

		// 未知错误

		return err
	})

	// 加密
	s.App.Use(func(c *fiber.Ctx) error {
		log.Println("加密")
		err := c.Next()
		log.Println("加密after")

		if err == nil {
			return err
		}

		if !s.config.EncEnable {
			return err
		}

		if str.HasPrefix(c.Path(), "/login") {
			return err
		}

		userPrincipal, ok := c.Context().Value("principal").(*UserPrincipal)
		if !ok {
			return NewErr("user not found")
		}

		var body any
		switch err.(type) {
		case *Text:
			log.Println("[将要加密文本]", err.Error())
			body = err.Error()
		case *Msg:
			js, _ := json.ToJson(err)
			log.Println("[将要加密JSON]", js)
			body = err
		default:
			// 未知错误
			return err
		}

		// 加密 转字符串
		encSs := ""
		switch body.(type) {
		case string:
		default:
			toJson, jsErr := json.ToJson(body)
			if jsErr != nil {
				return jsErr
			}
			encSs = toJson
		}
		encSs, encErr := encStr(userPrincipal, encSs)
		if encErr != nil {
			log.Println("[enc]加密错误")
			return err
		}
		body = encSs

		// 标记加密头
		c.Response().Header.Set("x-enc", "1")

		return NewText(body.(string))
	})

	// 错误转换
	s.App.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		if e, ok := err.(*fiber.Error); ok {
			if e.Code == http.StatusMethodNotAllowed {
				err = NewErr("Method Not Allowed")
			}
		} else if err == NotFound { // 不存在
			err = NewErr(err.Error())
		} else if err == NotAuthorized { // 无权限
			err = NewErr(err.Error())
		}
		return err
	})

	// 认证
	s.App.Use(func(c *fiber.Ctx) error {
		if !s.config.AuthEnable {
			return c.Next()
		}

		// 地址匹配
		path := c.Path()
		if len(path) > 0 {
			if str.HasPrefix(path, "/login/") {
				return c.Next()
			}
			if len(s.ignoreUrls) > 0 {
				for _, url := range s.ignoreUrls {
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
			return NewErr("use x-sign, but secret not found")
		}

		xSign := c.Get("x-sign")
		if str.IsBlank(xSign) {
			return NewErr("x-sign err")
		}

		// 取内容
		ss := ""
		switch c.Method() {
		case http.MethodGet:
			qs := c.Request().URI().QueryString()
			if len(qs) == 0 {
				return NewErr("qs err")
			}
			ss = string(qs)
		case http.MethodPost:
			body := c.Body()
			if len(body) > 0 {
				body = bytes.TrimUint8(body, 34) // 34:双引号
			}
			if len(body) == 0 {
				return NewErr("body err")
			}
			ss = string(body)
		}

		log.Println("[sign]字符串", ss)
		log.Println("[sign]签名", xSign)
		if !sign.CheckStr(ss, principal.Secret, xSign) {
			return NewErr("sign err")
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
			return NewErr("use x-enc, but not found principal")
		}
		if principal.Secret == "" {
			return NewErr("use x-enc, but secret not found")
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
					return NewErr("dec err")
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
					return NewErr("dec err")
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
			return c.JSON(NewErr("InternalServerError"))
		}}
	fa := fiber.New(config)
	return fa
}
