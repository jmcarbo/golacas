package main

import (
	"github.com/kataras/iris"
	"github.com/valyala/fasthttp"
	"github.com/jmcarbo/ldap"
	"time"
	"fmt"
	"flag"
	"strings"
	"math/rand"
	"sync"
	"github.com/jmcarbo/golacas/templates"
	"github.com/robfig/cron"
	"github.com/gorilla/securecookie"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
    letterIdxBits = 6                    // 6 bits to represent a letter index
    letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
    letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())
var hashKey = []byte("very-secret")
var blockKey = []byte("0123456789123456")
var secure = securecookie.New(hashKey, blockKey)

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
	encoded_value, err := secure.Encode(cookieName, tgt.Value)
	if err != nil {
		ctx.Log("Error encoding cookie %v", err)
	}	
	ctx.Log("Ticket: %s", tgt.Value)
	cookie.SetValue(encoded_value)
	cookie.SetPath(*basePath)
	ctx.SetCookie(&cookie)
}

func GetTGC(ctx *iris.Context) *Ticket {
	payload := ctx.GetCookie(cookieName)
	var decoded_value string
	secure.Decode(cookieName, payload, &decoded_value)
	ctx.Log("Ticket: %s", decoded_value)
	return GetTicket(decoded_value)
}

func DeleteTGC(ctx *iris.Context)  {
	ticket := GetTGC(ctx)
	if ticket!=nil {
		DeleteTicket(ticket.Value)
	}

	var cookie fasthttp.Cookie
	cookie.SetKey(cookieName)
	cookie.SetValue("deleted")
	cookie.SetPath(*basePath)
	ctx.SetCookie(&cookie)

	return
}

var (
	basePath = flag.String("basepath", "", "basepath")
	usetls = flag.Bool("usetls", false, "use https prefix")
	tickets = map[string]Ticket{}
	cookieName = "TGCGOLACAS"
	mutex = &sync.Mutex{}
	port = flag.String("port", "8080", "CAS listening port")
	ldapServer = flag.String("ldap", "localhost:389", "LDAP server")
	domain = flag.String("domain", "", "LDAP domain")
	garbageCollectionPeriod = 5
)



func main() {
	cr := cron.New()
	cr.AddFunc(fmt.Sprintf("@every %dm", garbageCollectionPeriod), collectTickets)
	cr.Start()
	flag.Parse()
	setApi()
	iris.Listen(":"+*port)
	cr.Stop()
}

func collectTickets() {
		fmt.Printf("Cleaning tickets\n")
		numTicketsCollected := 0
		m5, _ :=time.ParseDuration(fmt.Sprintf("%dm",garbageCollectionPeriod))
		five := time.Now().Add(-m5)
		mutex.Lock()
		for k, v := range tickets {
			if (v.Class == "ST") && v.CreatedAt.Before(five) {
				delete(tickets, k)
				numTicketsCollected++
			}
		}
		mutex.Unlock()
		fmt.Printf("%d tickets cleaned\n", numTicketsCollected)
}

func setApi() {
	if *basePath != "" {
		api := iris.Party(*basePath)
		api.Get("/login", login)("login")
		api.Post("/login", loginPost)
		api.Get("/logout", logout)("logout")
		api.Get("/validate", validate)("validate")
	} else {
		iris.Get("/login", login)("login")
		iris.Post("/login", loginPost)
		iris.Get("/logout", logout)("logout")
		iris.Get("/validate", validate)("validate")
	}

}

func getLocalURL(c *iris.Context) string {
	proto := "http"
	if c.RequestCtx.IsTLS() || *usetls {
		proto = "https"
	}
	return proto + "://" + c.HostString() 
}

func login(c *iris.Context) {
	service := c.URLParam("service")
	tgc := GetTGC(c)
	if tgc != nil {
		localservice := getLocalURL(c) + iris.Path("login")
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
	c.HTML(200, templates.HtmlHeader()+templates.BodyHeader()+
	  templates.FlashMessages(flash)+
	  templates.LoginForm(lt.Value)+
	  templates.BodyFooter()+templates.HtmlFooter())
}

func validateUser(username, password string) bool {

	if username == "" {
		return false
	}

	if *domain != "" && !strings.Contains(username, *domain) {
		username = username + "@" + *domain
	}

	c, err := ldap.Dial(*ldapServer)
	if err != nil {
		fmt.Println(err)
		return false
	}
	err = c.Bind(username, password)
	if err != nil {
		fmt.Println(err)
		return false
	}
	return true
}

func loginPost(c *iris.Context) {
	service := c.URLParam("service")
	username := c.FormValueString("username")
	password := c.FormValueString("password")

	if service == "" {
		service = getLocalURL(c) + iris.Path("login")
	}
	if !validateUser(username, password) {
		c.SetFlash("login_status", "ERROR. User unknown or incorrect password")
		fmt.Println("Validateuser false")
		service = getLocalURL(c) + iris.Path("login")
	} else {

		c.SetFlash("login_status", "User validation succeeded")
		fmt.Println("Validateuser true")
		st := NewTicket("ST", service, username, true)
		NewTGC(c, st)
		service = service + "?ticket=" + st.Value 
	}
	//c.Redirect(service, 303)
	c.HTML(200,  templates.Redirect(service) )
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
