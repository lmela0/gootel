# gootel
golang opentelemetry instrumentation library reduce setup effort

<div style="display: flex; justify-content: center; gap: 10px;">
  <img src="img/gootel_sized.png" alt="gootel logo" style="width: 21%; height: auto;">
  <img src="img/gonfigurator_sized.png" alt="gonfigurator logo" style="width: 20%; height: auto;">
</div>

**GOOtel** [ /'gu:tɛl/  a.k.a. **gutel** ] 

### Minimal library interface export
  - NewGOOtel(ctx context.Context, config *GOOtelConfig) (*GOOtel, error)
    - which returns a pointer to a GOOtel structure
  - type GOOtel struct
    - lib entry
  - type GOOtelConfig struct
    - lib config, abstracted away by GOOtel constructor
  - type QuantitativeMetric struct
    - metric wrapper holding all necessary info
  - type MultiHandler struct
    - Composite-CoR over multiple standard slog loggers
    
**GOOtel** helps you to organize the code in a single point to ease the interactions with OTLP exporters and to manage the OpenTelemetry library dependencies in a single point of interaction throughout the your whole project.
This allow you to exploit standard open telemetry framework in a simplified manner and use it in every single project to enhance observability even in small projects.

includes **gootel/configurator** library, 
here presented with a sample data mapping organized over different datatypes
  - string   *\``map:"SERVICE_NAME"`\`*
    - load data in string var if found in the provided load option
  - []string *\``map:"CORS_ORIGINS"`\`*
    - expect string in csv format to load slice
  - int      *\``map:"Database.MaxConnections"`\`*
    - load data in hierarchical JSON datastructure
  - int      *\``map:"Server.Port"`\`*
    - load data in hierarchical JSON datastructure

datastructure **GOOtelConfig** has its method *LoadWithDefaults* that load the structure with either mapped data or the defaults data for each member of config.

Hierarchical data structures are currently allowed only using JSON option.
If you want to map hierarchical data structure (referred with "." accessor in hierarchy) you need to use the same **map** tag that you use to map single values to values in data structure.
In case of hierarchical structures, you need to map only single values of structure and not the whole structure.

i.e.

```go
type DatabaseConfig struct {
	Host    string         `map:"Database.Host"`
	Max     int            `map:"Database.MaxConnections"`
}
type Config struct {
	Port    int            `map:"Server.Port"`
	Debug   bool           `map:"Server.Debug"`
	DB      DatabaseConfig // nested -> no map tag, recursively handled
 	Users   []string       `map:"AllowedUsers"` // handle CSV-like data like "data1,data2 ..., dataN"
 }
```

# How to use library

### gootel 
simple initialization and use in sample code: all configurations are mappend to default OTEL environment variables

```go
import (
	"github.com/lmela0/gootel"
)

func main() {
    // omissis...
	gootelCfg := gootel.GOOtelConfig{}
	tel, err := gootel.NewGOOtel(ctx, &gootelCfg)
	if err != nil {
		log.Fatal("cannot load gootel config:", err)
	}
	defer func() {
		tel.Shutdown()
	}()
    // omissis...
}
```

### gootel/gonfigurator

Given an example env i.e.
```bash
export OTEL_SERVICE_NAME="my-service"                        
export OTEL_SERVICE_VERSION="0.1"  
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT="http://logs-collector:4317"
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="http://collector:4317"         
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="http://metrics-collector:4317"
```

You can use **gonfigurator** a low footprint reflective companion package to fill your datastructure using *EnvOption* to fetch data from local machine environment

```go
func main() {
    // simple as unnecessary reference to GOOtelConfig as structure to be filled
	cfg := gootel.GOOtelConfig{}
    // fetch from env
	envOption := gonfigurator.EnvOption{}
	cfg.LoadWithDefaults(envOption)

	fmt.Printf("Config loaded:\n")
	fmt.Printf("\tServiceName: %s\n", cfg.ServiceName)
	fmt.Printf("\tServiceVersion: %s\n", cfg.ServiceVersion)
	fmt.Printf("\tLogsEndpoint: %s\n", cfg.LogsEndpoint)
	fmt.Printf("\tTracesEndpoint: %s\n", cfg.TracesEndpoint)
	fmt.Printf("\tMetricsEndpoint: %s\n", cfg.MetricsEndpoint)
}
// EXPECTED OUTPUT

// Config loaded:
//     ServiceName: my-service
//     ServiceVersion: 0.1
//     LogsEndpoint: http://logs-collector:4317
//     TracesEndpoint: http://collector:4317
//     MetricsEndpoint: http://metrics-collector:4317
```

Another example of using gonfigurator is related to *JsonOption*

Given the following datastructures


```go
type Config struct {
	Port  int            `map:"Server.Port"`
	Debug bool           `map:"Server.Debug"`
	DB    DatabaseConfig // nested
	Users []string       `map:"AllowedUsers"`
}
type DatabaseConfig struct {
	Host string `map:"Database.Host"`
	Max  int    `map:"Database.MaxConnections"`
}
```

and this **JSON object** saved in file "**config.json**"
```json
{
  "Server": {
    "Port": 8080,
    "Debug": true
  },
  "Database": {
    "Host": "localhost",
    "MaxConnections": 10
  },
  "AllowedUsers": ["user1", "user2"]
}
```

You can easily load data in this way

```go
// ...
	cfg := Config{}
	jsonOption := gonfigurator.JsonOption{ Path: "config.json" }
	if err := gonfigurator.LoadConfig(&cfg, jsonOption); err != nil {
		// handle config loading error
	}

	fmt.Printf("Config loaded:\n")
	fmt.Printf("\tServer.Port: %d\n", cfg.Port)
	fmt.Printf("\tServer.Debug: %v\n", cfg.Debug)
	fmt.Printf("\tAllowedUsers: %v\n", cfg.Users)
	fmt.Printf("\tDatabase.Host: %s\n", cfg.DB.Host)
	fmt.Printf("\tDatabase.MaxConnections: %d\n", cfg.DB.Max)
// ...

// EXPECTED OUTPUT

// Config loaded:
//     Server.Port: 8080
//     Server.Debug: true
//     AllowedUsers: [[user1 user2]]
//     Database.Host: localhost
//     Database.MaxConnections: 10
```