using UnrealBuildTool;

public class GameDepotUE : ModuleRules
{
    public GameDepotUE(ReadOnlyTargetRules Target) : base(Target)
    {
        PCHUsage = ModuleRules.PCHUsageMode.UseExplicitOrSharedPCHs;

        PublicDependencyModuleNames.AddRange(new string[]
        {
            "Core",
            "CoreUObject",
            "Engine",
            "AssetRegistry"
        });

        PrivateDependencyModuleNames.AddRange(new string[]
        {
            "UnrealEd",
            "LevelEditor",
            "ToolMenus",
            "Projects",
            "Slate",
            "SlateCore",
            "ContentBrowser",
            "EditorFramework",
            "InputCore",
            "HTTP",
            "Json",
            "JsonUtilities"
        });
    }
}
