module github.com/dat267/min

go 1.26.5

require (
	github.com/alecthomas/kong v1.16.0
	gopkg.in/yaml.v3 v3.0.1
)

retract [v0.1.152, v0.1.165] // Auto-tagged versions, not valid releases
