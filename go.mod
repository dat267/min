module github.com/dat267/min

go 1.26.5

require (
	github.com/alecthomas/kong v1.16.0
	gopkg.in/yaml.v3 v3.0.1
)

retract [v0.0.0, v0.1.165] // All versions before v0.1.166 are invalid
