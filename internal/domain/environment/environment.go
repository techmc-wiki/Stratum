package environment

type Loader string

const LoaderFabric Loader = "fabric"

type Environment struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	MinecraftVersion string   `json:"minecraftVersion"`
	JavaVersion      string   `json:"javaVersion"`
	Loader           Loader   `json:"loader"`
	ServerCore       string   `json:"serverCore"`
	MCDRConfigRef    string   `json:"mcdrConfigRef"`
	CarpetType       string   `json:"carpetType"`
	BaseMods         []string `json:"baseMods"`
	LucyManifestRef  string   `json:"lucyManifestRef"`
	LucyLockRef      string   `json:"lucyLockRef"`
}

func MVP117Fabric() Environment {
	return Environment{ID: "minecraft-1.17-fabric", Name: "Minecraft 1.17 Fabric + MCDR + Carpet", MinecraftVersion: "1.17", JavaVersion: "16", Loader: LoaderFabric, ServerCore: "fabric-server", MCDRConfigRef: "environments/1.17/mcdr", CarpetType: "fabric-carpet", BaseMods: []string{"fabric-api", "fabric-carpet"}, LucyManifestRef: "environments/1.17/lucy.json", LucyLockRef: "environments/1.17/lucy.lock"}
}
