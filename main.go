package main

import (
	"fmt"
	"gateway/breaker"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"syscall"
)

var (
	httpAddr      = EnvString("HTTP_ADDR", "0.0.0.0:8080")
	serviceRoutes = map[string]string{
		"/svc1": "http://0.0.0.0:8090",
		"/svc2": "http://0.0.0.0:8100",
	}
	serviceBreakers = make(map[string]*breaker.Breaker)
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.statusCode = status
	r.ResponseWriter.WriteHeader(status)
}

func main() {

	for path, _ := range serviceRoutes {
		b, err := breaker.NewBreaker(0.5)
		if err != nil {
			log.Fatal(err.Error())
		}

		serviceBreakers[path] = b
	}

	mux := http.NewServeMux()

	for path, svcUrl := range serviceRoutes {
		targetURL, err := url.Parse(svcUrl)
		if err != nil {
			log.Println("[ERROR]: Could not parse URL for %s: %v", path, err)
			continue
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)

		circuitBreakerHandler := createCircuitBreakerHandler(path, proxy)
		mux.Handle(path+"/", http.StripPrefix(path, circuitBreakerHandler))

	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "API Gateway is healthy")
	})
	mux.HandleFunc("/breaker-status", getBreakerStatus)

	log.Printf("API gateway running on %s", httpAddr)
	log.Fatal(http.ListenAndServe(httpAddr, mux))
}

func createCircuitBreakerHandler(servicePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := serviceBreakers[servicePath]

		if !b.AllowFlow() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Service temporarily unavailable. Circuit is OPEN for %s", servicePath)
			return
		}

		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default to 200 if not explicitly set
		}

		next.ServeHTTP(recorder, r)
		b.UpdateStatus(recorder.statusCode)
		log.Printf("[%s] Status: %d, Circuit: %v, Failure rate: %.2f%%",
			servicePath, recorder.statusCode, b.CurrentStatus, b.GetFailedRequests()*100)
	})
}

func getBreakerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n")

	i := 0
	for path, b := range serviceBreakers {
		statusStr := "CLOSED"
		if b.CurrentStatus == breaker.OPEN {
			statusStr = "OPEN"
		} else if b.CurrentStatus == breaker.HALF_OPEN {
			statusStr = "HALF_OPEN"
		}

		fmt.Fprintf(w, "  \"%s\": {\n", path)
		fmt.Fprintf(w, "    \"status\": \"%s\",\n", statusStr)
		fmt.Fprintf(w, "    \"failureRate\": %.2f,\n", b.GetFailedRequests()*100)
		fmt.Fprintf(w, "    \"requestCount\": %d,\n", b.RequestCounter)
		fmt.Fprintf(w, "    \"successCount\": %d\n", b.SuccessCounter)

		if i < len(serviceBreakers)-1 {
			fmt.Fprintf(w, "  },\n")
		} else {
			fmt.Fprintf(w, "  }\n")
		}
		i++
	}

	fmt.Fprintf(w, "}\n")
}

func EnvString(key, fallback string) string {
	if val, ok := syscall.Getenv(key); ok {
		return val
	}
	return fallback
}

func getPostgresConnection() (*pgx.Conn, error) {
	// Define the connection string (change with your actual connection string)
	connConfig := "postgres://username:password@localhost:5432/dbname"

	// Connect to the PostgreSQL database
	conn, err := pgx.Connect(context.Background(), connConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	fmt.Println("Successfully connected to the database!")

	return conn, nil
}
