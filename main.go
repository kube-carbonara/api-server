package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	gHandler "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/kube-carbonara/api-server/connections"
	"github.com/kube-carbonara/api-server/controllers"
	handlers "github.com/kube-carbonara/api-server/handlers"
	"github.com/kube-carbonara/api-server/models"
	"github.com/kube-carbonara/api-server/routers"
	"github.com/kube-carbonara/api-server/ws"
	"github.com/rancher/remotedialer"
	"github.com/sirupsen/logrus"
)

func init() {
}

func authorizer(req *http.Request) (string, bool, error) {
	id := req.Header.Get("x-tunnel-id")
	return id, id != "", nil
}

func main() {
	godotenv.Load()

	var (
		addr      string
		peerID    string
		peerToken string
		peers     string
		debug     bool
	)
	flag.StringVar(&addr, "listen", ":8099", "Listen address")
	flag.StringVar(&peerID, "id", "", "Peer ID")
	flag.StringVar(&peerToken, "token", "", "Peer Token")
	flag.StringVar(&peers, "peers", "", "Peers format id:token:url,id:token:url")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		remotedialer.PrintTunnelData = true
	}
	handler := remotedialer.New(authorizer, remotedialer.DefaultErrorWriter)
	handler.PeerToken = peerToken
	handler.PeerID = peerID
	for _, peer := range strings.Split(peers, ",") {
		parts := strings.SplitN(strings.TrimSpace(peer), ":", 3)
		if len(parts) != 3 {
			continue
		}
		handler.AddPeer(parts[2], parts[0], parts[1])
	}

	Insector := connections.ClusterInsector{Dialer: handler}

	go Insector.OnStartUp()

	router := mux.NewRouter()
	router.Handle("/connect", handler)

	client := &ws.Client{
		Messages: make(chan []byte),
	}

	client.Connections = make(map[*ws.Hub]bool)
	router.HandleFunc("/monitoring", func(rw http.ResponseWriter, r *http.Request) {
		client.ServeMonitoring(rw, r)
	})

	router.HandleFunc("/outbound", func(rw http.ResponseWriter, r *http.Request) {
		client.ServeOutBound(rw, r)
	})

	if os.Getenv("DEFAULT_USER_NAME") != "" && os.Getenv("DEFAULT_PASSWORD") != "" {
		controllers.UsersController{}.CreateDefaultUser(models.Users{
			UserName: os.Getenv("DEFAULT_USER_NAME"),
			Password: os.Getenv("DEFAULT_PASSWORD"),
			IsAdmin:  true,
		})
	}

	go client.ReadMessage()
	routers.ClusterRouter{}.Handle(router)
	routers.AuthRouter{}.Handle(router)
	routers.UserRouter{}.Handle(router)
	router.HandleFunc("/health", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("up is running"))
	})

	router.HandleFunc("/healthaknowlegment", func(rw http.ResponseWriter, r *http.Request) {
		Insector.Aknowlegement(rw, r, 40*time.Second)
	})
	router.HandleFunc("/connections/{id}/{path:.*}", func(rw http.ResponseWriter, req *http.Request) {
		handlers.ClientHandler{}.Handle(handler, rw, req)
	})
	fmt.Println("Listening on ", addr)

	log.Fatal(http.ListenAndServe(addr, gHandler.CORS(gHandler.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}), gHandler.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS"}),
		gHandler.AllowedOrigins([]string{"*"}))(router)))

}
