package workshopbackend

var (
	MergeInstancesAndFiles = mergeInstancesAndFiles
	LoadWorkshop           = (*LxdBackend).loadWorkshop
	LxdDevices             = (*Device).lxdProperties
	ReadWorkshop           = readWorkshop
	ReadProjects           = readProjects
	SaveProjects           = saveProjects
)
