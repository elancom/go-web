package web

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/elancom/go-util/crypto"
	"github.com/elancom/go-util/lang"
	"github.com/elancom/go-util/param"
	"github.com/elancom/go-util/rand"
	"github.com/elancom/go-util/str"
	"github.com/gofiber/fiber/v2"
	"time"
)

type UserPrincipal struct {
	Id        int64  `json:"id"`        // 用户ID
	Username  string `json:"username"`  // 用户名
	Key       string `json:"key"`       // 唯一标识
	Secret    string `json:"secret"`    // 通信秘钥(16位)
	Random    string `json:"random"`    // 随机字符串
	Timestamp int64  `json:"timestamp"` // 时间戳(毫秒)
}

func MakeToken(id int64, username string, secret string, aesKey []byte) (string, error) {
	principal := UserPrincipal{
		Id:        id,
		Username:  username,
		Key:       crypto.NewId32(),
		Secret:    secret,
		Random:    rand.RandomStr(32),
		Timestamp: time.Now().UnixMilli(),
	}
	marshal, err := json.Marshal(principal)
	if err != nil {
		return "", err
	}

	encrypt, err := crypto.AesEcbEncrypt(marshal, aesKey)
	if err != nil {
		return "", errors.New("encrypt(0)")
	}

	enStr := base64.StdEncoding.EncodeToString(encrypt)
	if err != nil {
		return "", errors.New("encrypt(es)")
	}

	return enStr, nil
}

func GetUserPrincipal(token string) (*UserPrincipal, error) {
	if str.IsBlank(token) {
		return nil, errors.New("token err")
	}

	tokenBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("token err(DC)")
	}

	decrypt, err := crypto.AesEcbDecrypt(tokenBytes, []byte("1234567890123456"))
	if err != nil {
		return nil, errors.New("token err(0)")
	}

	principal := new(UserPrincipal)
	err = json.Unmarshal(decrypt, principal)
	if err != nil {
		return nil, errors.New("token err(1)")
	}

	if principal.Id == 0 || str.IsBlank(principal.Username) || str.IsBlank(principal.Key) || principal.Timestamp == 0 {
		return nil, errors.New("token err(2)")
	}

	return principal, nil
}

func parseUserPrincipal(c *fiber.Ctx) (*UserPrincipal, error) {
	token := c.Get("x-token")
	token = str.Trim(token)

	if str.IsBlank(token) {
		return nil, lang.NewErr("token err(B)")
	}

	principal, err := GetUserPrincipal(token)
	if err != nil {
		return nil, c.JSON(lang.NewErr(err.Error()))
	}

	return principal, err
}

type HandleWithUser func(principal *UserPrincipal) error

// ResolveUser 用户解析
func ResolveUser(c *fiber.Ctx) (*UserPrincipal, error) {
	if principal, ok := c.Context().Value("principal").(*UserPrincipal); ok {
		return principal, nil
	}
	return nil, lang.NewErr("principal error")
}

// UseUser 注入用户
func UseUser(handle HandleP1[*UserPrincipal]) fiber.Handler {
	return Bind1(handle, ResolveUser)
}

// UseUserParam UseUser 注入用户
func UseUserParam(handle HandleP2[*UserPrincipal, string], name string) fiber.Handler {
	return Bind2(handle, ResolveUser, ResolveParam(name))
}

// UseUserParam2 UseUser 注入用户
func UseUserParam2(handle HandleP3[*UserPrincipal, string, string], name1 string, name2 string) fiber.Handler {
	creator := newParamFuncCreator()
	return Bind3(handle, ResolveUser, creator(name1), creator(name2))
}

// UseUserParam3 UseUser 注入用户
func UseUserParam3(handle HandleP4[*UserPrincipal, string, string, string], name1 string, name2 string, name3 string) fiber.Handler {
	creator := newParamFuncCreator()
	return Binds(handle, ResolveUser, creator(name1), creator(name2), creator(name3))
}

// UseUserBody UseUser 注入用户
func UseUserBody[T any](handle HandleP2[*UserPrincipal, T], supplier Supplier[T]) fiber.Handler {
	return Bind2(handle, ResolveUser, ResolveBody[T](supplier))
}

// UseUserPageParams UseUser 注入用户
func UseUserPageParams(handle HandleP3[*UserPrincipal, *lang.Page, *param.Params]) fiber.Handler {
	return Bind3(handle, ResolveUser, ResolvePage, ResolveParams)
}

// UseUserPageParam UseUser 注入用户
func UseUserPageParam(handle HandleP3[*UserPrincipal, *lang.Page, string], name string) fiber.Handler {
	return Bind3(handle, ResolveUser, ResolvePage, ResolveParam(name))
}

// UseUserPageParam2 UseUser 注入用户
func UseUserPageParam2(handle HandleP4[*UserPrincipal, *lang.Page, string, string], name1 string, name2 string) fiber.Handler {
	creator := newParamFuncCreator()
	return Binds(handle, ResolveUser, ResolvePage, creator(name1), creator(name2))
}

// UseUserPageBody UseUser 注入用户
func UseUserPageBody[T any](handle HandleP3[*UserPrincipal, *lang.Page, T], supplier Supplier[T]) fiber.Handler {
	return Bind3(handle, ResolveUser, ResolvePage, ResolveBody[T](supplier))
}

// ResolveOptUser 用户解析(可选)
func ResolveOptUser(c *fiber.Ctx) (*UserPrincipal, error) {
	user, err := ResolveUser(c)
	if err != nil {
		return parseUserPrincipal(c)
	}
	return user, nil
}

// UseOptUser 注入用户(可选)
func UseOptUser(handle HandleP1[*UserPrincipal]) fiber.Handler {
	return Bind1(handle, ResolveOptUser)
}

// UseUserWithIsCount 注入用户,统计
func UseUserWithIsCount(fn HandleP2[*UserPrincipal, bool]) fiber.Handler {
	return Bind2(func(p1 *UserPrincipal, p2 bool) error { return fn(p1, p2) }, ResolveUser, ResolveIsCount)
}
