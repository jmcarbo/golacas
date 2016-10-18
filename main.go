package main

import (
	"github.com/kataras/iris"
	"github.com/valyala/fasthttp"
	"time"
	"math/rand"
	"sync"
	"github.com/jmcarbo/golacas/templates"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
    letterIdxBits = 6                    // 6 bits to represent a letter index
    letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
    letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

func RandString(n int) string {
    b := make([]byte, n)
    // A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
    for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
        if remain == 0 {
            cache, remain = src.Int63(), letterIdxMax
        }
        if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
            b[i] = letterBytes[idx]
            i--
        }
        cache >>= letterIdxBits
        remain--
    }

    return string(b)
}

type Ticket struct {
	Class string
	Value string
	User string
	Service string
	CreatedAt time.Time
	Renew bool
}

func NewTicket(class string, service string, user string, renew bool) *Ticket {
	t := Ticket{ Class: class, Value: class+RandString(128), CreatedAt: time.Now(), User: user, Service: service, Renew: renew }
	mutex.Lock()
	tickets[t.Value]=t	
	mutex.Unlock()
	return &t
}

func GetTicket(value string) *Ticket {
	mutex.Lock()
	t, ok := tickets[value]
	mutex.Unlock()
	if ok {
		return &t
	} else {
		return nil
	}
}

func DeleteTicket(value string)  {
	mutex.Lock()
	delete(tickets,value)
	mutex.Unlock()
}

func NewTGC(ctx *iris.Context, ticket *Ticket) {
	var cookie fasthttp.Cookie
	cookie.SetKey(cookieName)
	tgt := NewTicket("TGT", ticket.Service, ticket.User, false)
	cookie.SetValue(tgt.Value)
	cookie.SetPath(basePath)
	ctx.SetCookie(&cookie)
}

func GetTGC(ctx *iris.Context) *Ticket {
	payload := ctx.GetCookie(cookieName)
	return GetTicket(payload) 
}

func DeleteTGC(ctx *iris.Context)  {
	ticket := GetTGC(ctx)
	if ticket!=nil {
		DeleteTicket(ticket.Value)
	}

	var cookie fasthttp.Cookie
	cookie.SetKey(cookieName)
	cookie.SetValue("deleted")
	cookie.SetPath(basePath)
	ctx.SetCookie(&cookie)

	return
}

var (
	basePath string = "/cas"
	tickets = map[string]Ticket{}
	cookieName = "TGCGOLACAS"
	mutex = &sync.Mutex{}
)



func main() {
	setApi()
	iris.Listen(":8080")
}

func setApi() {
	api := iris.Party(basePath)
	api.Get("/login", login)("login")
	api.Post("/login", loginPost)
	api.Get("/logout", logout)("logout")
	api.Get("/validate", validate)("validate")

}

func login(c *iris.Context) {
	service := c.URLParam("service")
	tgc := GetTGC(c)
	if tgc != nil {
		localservice := "http://"+ c.HostString() + iris.Path("login")
		st := NewTicket("ST", service, tgc.User, false)
		c.SetFlash("login_status", "User validation succeeded")
		if service != "" && service != localservice {
			service = service + "?ticket=" + st.Value 
			c.HTML(200, templates.Redirect(service))
			return
		}
		service = localservice + "?ticket=" + st.Value 
	}

	lt := NewTicket("LT", "", "", false)
	flash, _ := c.GetFlash("login_status")
	c.HTML(200, templates.HtmlHeader()+
	  templates.FlashMessages(flash)+
	  templates.LoginForm(lt.Value)+
	  templates.HtmlFooter())
}

func loginPost(c *iris.Context) {
	service := c.URLParam("service")
	username := c.FormValueString("username")
	password := c.FormValueString("password")

	if service == "" {
		service = "http://"+ c.HostString() + iris.Path("login")
	}
	if username == "" || username != password {
		c.SetFlash("login_status", "ERROR. User unknown or incorrect password")
	} else {
		c.SetFlash("login_status", "User validation succeeded")
		st := NewTicket("ST", service, username, true)
		NewTGC(c, st)
		service = service + "?ticket=" + st.Value 
	}
	//c.Redirect(service, 303)
	c.HTML(200, templates.Redirect(service))
}

func logout(c *iris.Context) {
	tgc := GetTGC(c)
	if tgc != nil {
		DeleteTGC(c)
		c.Write("User has been logged out")
	} else  {
		c.Write("User is not logged in")
	}	
}

func validate(c *iris.Context) {
	service := c.URLParam("service")
	ticket := c.URLParam("ticket")

	if ticket == "" {
		c.Write("no\n")
	} else {
		t := GetTicket(ticket)
		if t == nil {
			c.Write("no\n")
		} else {
			if t.Service != service {
				c.Write("no\n")
			} else {
				DeleteTicket(ticket)
				c.Write("yes\n" + t.User)
			}
		}
	}
}
