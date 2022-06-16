package server

import (
	"github.com/elancom/go-util/collection"
	"github.com/elancom/go-util/lang"
	"github.com/elancom/go-util/number"
	"github.com/elancom/go-util/param"
	"github.com/elancom/go-util/str"
	"github.com/gofiber/fiber/v2"
	"net/http"
)

// 参数绑定

type Supplier[T any] func() T

type Handle func() error
type HandleP1[T1 any] func(p1 T1) error
type HandleP2[T1 any, T2 any] func(p1 T1, p2 T2) error
type HandleP3[T1 any, T2 any, T3 any] func(p1 T1, p2 T2, p3 T3) error
type HandleP4[T1 any, T2 any, T3 any, T4 any] func(p1 T1, p2 T2, p3 T3, p4 T4) error

type HandleWithParams func(params *param.Params) error
type HandleWithForm func(form *param.Params) error

type HandleWithString func(params string) error
type HandleWithInt[T int | int64] func(i T) error

// Resolver 参数解析器
type Resolver[T any] func(c *fiber.Ctx) (T, error)

// Nil参数解析
var none Resolver[any] = func(c *fiber.Ctx) (any, error) { return nil, nil }

func ResolveInt[T int | int64](name string) func(c *fiber.Ctx) (T, error) {
	return func(c *fiber.Ctx) (T, error) {
		p := ""
		if c.Method() == http.MethodPost {
			p = c.Params(name)
		} else {
			p = c.Query(name)
		}
		if str.IsBlank(p) {
			return T(0), lang.NewErr(name + " missing")
		}
		return number.ToInt[T](p)
	}
}

// UseInt 注入参数
func UseInt(handler HandleWithInt[int], name string) fiber.Handler {
	return Bind1(func(p1 int) error { return handler(p1) }, ResolveInt[int](name))
}

// UseInt64 注入参数
func UseInt64(handler HandleWithInt[int64], name string) fiber.Handler {
	return Bind1(func(p1 int64) error { return handler(p1) }, ResolveInt[int64](name))
}

// UseId64 注入参数
func UseId64(handler HandleWithInt[int64]) fiber.Handler {
	return Bind1(func(p1 int64) error { return handler(p1) }, ResolveInt[int64]("id"))
}

// ResolvePathVar 路由参数解析
func ResolvePathVar(c *fiber.Ctx) (*param.Params, error) {
	if c.Method() == http.MethodPost {
		return param.NewParams(c.AllParams()), nil
	}
	return param.NewParams(map[string]string{}), nil
}

// UsePathVar  注入路由参数
func UsePathVar(handler HandleWithParams) fiber.Handler {
	return Bind1(func(p1 *param.Params) error { return handler(p1) }, ResolvePathVar)
}

// ResolveForm 表单参数解析
func ResolveForm(c *fiber.Ctx) (*param.Params, error) {
	if c.Method() == http.MethodPost {
		args := c.Request().PostArgs()
		m := make(map[string]string, args.Len())
		args.VisitAll(func(key, value []byte) { m[string(key)] = string(value) })
		return param.NewParams(m), nil
	}
	return param.NewParams(map[string]string{}), nil
}

// UseForm 注入表单参数
func UseForm(handler HandleWithForm) fiber.Handler {
	return Bind1(func(p1 *param.Params) error { return handler(p1) }, ResolveForm)
}

func ResolveParams(c *fiber.Ctx) (*param.Params, error) {
	m, err := make(map[string]string), error(nil)
	switch c.Method() {
	case http.MethodPost:
		// todo mb a bug
		err = c.BodyParser(&m)
	case http.MethodGet:
		args := c.Request().URI().QueryArgs()
		args.VisitAll(func(key, value []byte) { m[string(key)] = string(value) })
	}
	if err != nil {
		return nil, err
	}
	return param.NewParams(m), nil
}

func ResolveParam(name string) Resolver[string] {
	return func(c *fiber.Ctx) (string, error) {
		params, err := ResolveParams(c)
		if err != nil {
			return "", err
		}
		return params.Get(name), nil
	}
}

// UseParam 1个参数
func UseParam(handle HandleP1[string], name string) fiber.Handler {
	return Bind1[string](handle, ResolveParam(name))
}

// UseParams 1个参数
func UseParams(handle HandleP1[*param.Params]) fiber.Handler {
	return Bind1[*param.Params](handle, ResolveParams)
}

// ResolveBody 解析body
// 支持post(json)/post_form(form_data)/get
func ResolveBody[T any](gen Supplier[T]) Resolver[T] {
	return func(c *fiber.Ctx) (T, error) {
		dist := gen()
		err := error(nil)
		switch c.Method() {
		case http.MethodPost:
			err = c.BodyParser(dist)
		case http.MethodGet:
			err = c.QueryParser(dist)
		default:
			err = lang.NewErr("not support use body")
		}
		if err != nil {
			return dist, err
		}
		return dist, nil
	}
}

// UseBody 注入body
func UseBody[T any](handle HandleP1[T], supplier Supplier[T]) fiber.Handler {
	return Bind1[T](handle, ResolveBody[T](supplier))
}

// Use 无参处理器
func Use(handle Handle) fiber.Handler {
	return Bind0(handle)
}

func ResolvePage(c *fiber.Ctx) (*lang.Page, error) {
	page, pv, prows := new(lang.Page), "", ""
	switch c.Method() {
	case http.MethodGet:
		gen := func(_ int, it string) string { return c.Query(it) }
		pv = collection.FindMapS2s([]string{"page", "current"}, gen)
		prows = collection.FindMapS2s([]string{"rows", "pageSize"}, gen)
	case http.MethodPost:
		m := make(map[string]string)
		err := c.BodyParser(m)
		if err == nil {
			gen := func(_ int, it string) string { return m[it] }
			pv = collection.FindMapS2s([]string{"page", "current"}, gen)
			prows = collection.FindMapS2s([]string{"rows", "pageSize"}, gen)
		}
	}
	if str.IsNotBlank(pv) {
		if i := number.ToIntL(pv, 0); i > 0 {
			page.SetPage(i)
		}
	}
	if str.IsNotBlank(prows) {
		if i := number.ToIntL(prows, 0); i > 0 {
			page.SetRows(i)
		}
	}
	return page, nil
}

// UsePage 分页对象
func UsePage(handle HandleP1[*lang.Page]) fiber.Handler {
	return Bind1(handle, ResolvePage)
}

// UsePageCount 分页对象
func UsePageCount(handle HandleP2[*lang.Page, bool]) fiber.Handler {
	return Bind2(handle, ResolvePage, ResolveIsCount)
}

// UsePageFlag 分页对象
func UsePageFlag(handle HandleP2[*lang.Page, *lang.Flag]) fiber.Handler {
	return Bind2(handle, ResolvePage, ResolveFlag)
}

// UsePageFlagParam 分页对象
func UsePageFlagParam(handle HandleP3[*lang.Page, *lang.Flag, string], name string) fiber.Handler {
	return Bind3(handle, ResolvePage, ResolveFlag, ResolveParam(name))
}

// UsePageFlagParams 分页对象
func UsePageFlagParams(handle HandleP3[*lang.Page, *lang.Flag, *param.Params]) fiber.Handler {
	return Bind3(handle, ResolvePage, ResolveFlag, ResolveParams)
}

func UsePageParams(handle HandleP2[*lang.Page, *param.Params]) fiber.Handler {
	return Bind2(handle, ResolvePage, ResolveParams)
}

func UsePageCountParams(handle HandleP3[*lang.Page, bool, *param.Params]) fiber.Handler {
	return Bind3(handle, ResolvePage, ResolveIsCount, ResolveParams)
}

func UsePageParam(handle HandleP2[*lang.Page, string], name string) fiber.Handler {
	return Bind2(handle, ResolvePage, ResolveParam(name))
}

func UsePageCountParam(handle HandleP3[*lang.Page, bool, string], name string) fiber.Handler {
	return Bind3(handle, ResolvePage, ResolveIsCount, ResolveParam(name))
}

// ResolveIsCount 是否统计查询解析
func ResolveIsCount(c *fiber.Ctx) (bool, error) {
	return str.HasSuffix(c.Path(), "/count"), nil
}

// ResolveFlag 是否统计查询解析
func ResolveFlag(c *fiber.Ctx) (*lang.Flag, error) {
	isList := str.HasSuffix(c.Path(), "/list")
	isCount := str.HasSuffix(c.Path(), "/count")
	isSummary := !isCount && str.HasSuffix(c.Path(), "/sum")
	f := new(lang.Flag)
	f.IsCount = isCount
	f.IsSummary = isSummary
	f.IsList = isList
	return f, nil
}

// Bind0 绑定1个参数
func Bind0(
	fn Handle,
) fiber.Handler {
	return Binds(func(p1 any, n2 any, n3 any, n4 any) error { return fn() }, none, none, none, none)
}

// Bind1 绑定1个参数
func Bind1[T1 any](
	fn HandleP1[T1],
	r1 Resolver[T1],
) fiber.Handler {
	return Binds(func(p1 T1, n2 any, n3 any, n4 any) error { return fn(p1) }, r1, none, none, none)
}

// Bind2 绑定2个参数
func Bind2[T1 any, T2 any](
	fn HandleP2[T1, T2],
	r1 Resolver[T1], r2 Resolver[T2],
) fiber.Handler {
	return Binds(func(p1 T1, p2 T2, n3 any, n4 any) error { return fn(p1, p2) }, r1, r2, none, none)
}

// Bind3 绑定3个参数
func Bind3[T1 any, T2 any, T3 any](
	fn HandleP3[T1, T2, T3],
	r1 Resolver[T1], r2 Resolver[T2], r3 Resolver[T3],
) fiber.Handler {
	return Binds(func(p1 T1, p2 T2, p3 T3, n4 any) error { return fn(p1, p2, p3) }, r1, r2, r3, none)
}

// Binds 绑定4个参数
func Binds[T1 any, T2 any, T3 any, T4 any](
	fn HandleP4[T1, T2, T3, T4],
	r1 Resolver[T1], r2 Resolver[T2], r3 Resolver[T3], r4 Resolver[T4],
) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 参数1
		p1, err := r1(c)
		if err != nil {
			return lang.NewErr("p1 resolve err")
		}

		// 参数2
		p2, err := r2(c)
		if err != nil {
			return lang.NewErr("p2 resolve err")
		}

		// 参数3
		p3, err := r3(c)
		if err != nil {
			return lang.NewErr("p3 resolve err")
		}

		// 参数4
		p4, err := r4(c)
		if err != nil {
			return lang.NewErr("p4 resolve err")
		}

		return fn(p1, p2, p3, p4)
	}
}
