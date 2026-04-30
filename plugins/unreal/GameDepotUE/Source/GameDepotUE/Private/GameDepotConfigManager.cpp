#include "GameDepotConfigManager.h"

#include "HAL/FileManager.h"
#include "Misc/FileHelper.h"
#include "Misc/Paths.h"

namespace
{
FString StripYamlQuotes(FString Value)
{
    Value = Value.TrimStartAndEnd();
    if ((Value.StartsWith(TEXT("\"")) && Value.EndsWith(TEXT("\""))) ||
        (Value.StartsWith(TEXT("'")) && Value.EndsWith(TEXT("'"))))
    {
        Value = Value.Mid(1, Value.Len() - 2);
    }
    return Value;
}

FString ReadRuleValue(const FString& Line, const FString& Key)
{
    const FString Prefix = Key + TEXT(":");
    FString Trimmed = Line.TrimStartAndEnd();
    if (!Trimmed.StartsWith(Prefix))
    {
        return FString();
    }
    return StripYamlQuotes(Trimmed.RightChop(Prefix.Len()));
}

FString EscapeRuleValue(const FString& Value)
{
    FString Escaped = Value;
    Escaped.ReplaceInline(TEXT("\\"), TEXT("\\\\"));
    Escaped.ReplaceInline(TEXT("\""), TEXT("\\\""));
    return FString::Printf(TEXT("\"%s\""), *Escaped);
}
}

FGameDepotConfigManager::FGameDepotConfigManager()
{
    ProjectRoot = FPaths::ConvertRelativePathToFull(FPaths::ProjectDir());
    DepotDir = FPaths::Combine(ProjectRoot, TEXT(".gamedepot"));
    ConfigPath = FPaths::Combine(DepotDir, TEXT("mock-ui-config.yaml"));
    RulesPath = FPaths::Combine(DepotDir, TEXT("mock-rules.yaml"));
    Snapshot = MakeDefaultSnapshot();
}

void FGameDepotConfigManager::Load()
{
    Snapshot = MakeDefaultSnapshot();
    Snapshot.bInitialized = FPaths::FileExists(ConfigPath);

    FString Content;
    if (FFileHelper::LoadFileToString(Content, *ConfigPath))
    {
        Snapshot.OSSProvider = ReadScalar(Content, TEXT("oss_provider"), Snapshot.OSSProvider);
        Snapshot.OSSEndpoint = ReadScalar(Content, TEXT("oss_endpoint"), Snapshot.OSSEndpoint);
        Snapshot.OSSBucket = ReadScalar(Content, TEXT("oss_bucket"), Snapshot.OSSBucket);
        Snapshot.OSSRegion = ReadScalar(Content, TEXT("oss_region"), Snapshot.OSSRegion);
        Snapshot.OSSPrefix = ReadScalar(Content, TEXT("oss_prefix"), Snapshot.OSSPrefix);
    }

    FString RuleContent;
    if (FFileHelper::LoadFileToString(RuleContent, *RulesPath))
    {
        Snapshot.RuleText = RuleContent;
        Snapshot.Rules = ParseRules(RuleContent);
        if (Snapshot.Rules.Num() == 0)
        {
            Snapshot.Rules = DefaultRules();
            Snapshot.RuleText = RulesToText(Snapshot.Rules);
        }
    }
    else
    {
        Snapshot.Rules = DefaultRules();
        Snapshot.RuleText = RulesToText(Snapshot.Rules);
    }
}

bool FGameDepotConfigManager::Save(const FGameDepotConfigSnapshot& NewSnapshot, FString& OutError)
{
    if (!IFileManager::Get().DirectoryExists(*DepotDir))
    {
        if (!IFileManager::Get().MakeDirectory(*DepotDir, true))
        {
            OutError = FString::Printf(TEXT("Failed to create %s"), *DepotDir);
            return false;
        }
    }

    FGameDepotConfigSnapshot ToSave = NewSnapshot;
    ToSave.bInitialized = true;
    ToSave.RuleText = RulesToText(ToSave.Rules);

    if (!FFileHelper::SaveStringToFile(ToConfigFileText(ToSave), *ConfigPath))
    {
        OutError = FString::Printf(TEXT("Failed to write %s"), *ConfigPath);
        return false;
    }

    if (!FFileHelper::SaveStringToFile(ToSave.RuleText, *RulesPath))
    {
        OutError = FString::Printf(TEXT("Failed to write %s"), *RulesPath);
        return false;
    }

    Snapshot = ToSave;
    return true;
}

bool FGameDepotConfigManager::InitializeDefault(FString& OutError)
{
    FGameDepotConfigSnapshot Defaults = MakeDefaultSnapshot();
    Defaults.bInitialized = true;
    Defaults.RuleText = RulesToText(Defaults.Rules);
    return Save(Defaults, OutError);
}

FText FGameDepotConfigManager::BuildValidationSummary() const
{
    TArray<FString> Issues;
    if (!Snapshot.bInitialized)
    {
        Issues.Add(TEXT("Workspace is not initialized."));
    }
    if (Snapshot.OSSBucket.IsEmpty())
    {
        Issues.Add(TEXT("OSS bucket is empty."));
    }
    if (Snapshot.OSSRegion.IsEmpty())
    {
        Issues.Add(TEXT("OSS region is empty."));
    }
    if (Snapshot.Rules.Num() == 0)
    {
        Issues.Add(TEXT("No rules configured."));
    }
    for (int32 Index = 0; Index < Snapshot.Rules.Num(); ++Index)
    {
        const FGameDepotRuleConfig& Rule = Snapshot.Rules[Index];
        if (Rule.Pattern.TrimStartAndEnd().IsEmpty())
        {
            Issues.Add(FString::Printf(TEXT("Rule %d pattern is empty."), Index + 1));
        }
        const FString Mode = Rule.Mode.ToLower();
        if (!(Mode == TEXT("git") || Mode == TEXT("blob") || Mode == TEXT("ignore") ))
        {
            Issues.Add(FString::Printf(TEXT("Rule %d mode should be git, blob, or ignore."), Index + 1));
        }
    }

    if (Issues.Num() == 0)
    {
        return FText::FromString(TEXT("Config looks valid for the mock UI. Real connectivity checks will be attached to the daemon later."));
    }
    return FText::FromString(FString::Join(Issues, TEXT("\n")));
}

FGameDepotConfigSnapshot FGameDepotConfigManager::MakeDefaultSnapshot()
{
    FGameDepotConfigSnapshot Defaults;
    Defaults.OSSProvider = TEXT("aliyun-oss");
    Defaults.OSSEndpoint = TEXT("oss-cn-hangzhou.aliyuncs.com");
    Defaults.OSSBucket = TEXT("your-game-bucket");
    Defaults.OSSRegion = TEXT("cn-hangzhou");
    Defaults.OSSPrefix = TEXT("gamedepot/blobs");
    Defaults.Rules = DefaultRules();
    Defaults.RuleText = RulesToText(Defaults.Rules);
    return Defaults;
}

TArray<FGameDepotRuleConfig> FGameDepotConfigManager::DefaultRules()
{
    TArray<FGameDepotRuleConfig> Rules;

    auto AddRule = [&Rules](const TCHAR* Pattern, const TCHAR* Mode, const TCHAR* Kind, const TCHAR* Scope)
    {
        FGameDepotRuleConfig Rule;
        Rule.Pattern = Pattern;
        Rule.Mode = Mode;
        Rule.Kind = Kind;
        Rule.Scope = Scope;
        Rules.Add(Rule);
    };

    AddRule(TEXT("Content/**/*.uasset"), TEXT("blob"), TEXT("unreal_asset"), TEXT("glob"));
    AddRule(TEXT("Content/**/*.umap"), TEXT("blob"), TEXT("unreal_map"), TEXT("glob"));
    AddRule(TEXT("Content/**/*.json"), TEXT("git"), TEXT("data_source"), TEXT("glob"));
    AddRule(TEXT("Config/**"), TEXT("git"), TEXT("config"), TEXT("glob"));
    AddRule(TEXT("Source/**"), TEXT("git"), TEXT("source"), TEXT("glob"));
    AddRule(TEXT("Saved/**"), TEXT("ignore"), TEXT("generated"), TEXT("glob"));
    return Rules;
}

TArray<FGameDepotRuleConfig> FGameDepotConfigManager::ParseRules(const FString& RuleContent)
{
    TArray<FGameDepotRuleConfig> Rules;
    TArray<FString> Lines;
    RuleContent.ParseIntoArrayLines(Lines, false);

    FGameDepotRuleConfig Current;
    bool bHasCurrent = false;

    auto FlushCurrent = [&]()
    {
        if (bHasCurrent && !Current.Pattern.TrimStartAndEnd().IsEmpty())
        {
            if (Current.Mode.IsEmpty()) Current.Mode = TEXT("git");
            if (Current.Kind.IsEmpty()) Current.Kind = TEXT("manual");
            if (Current.Scope.IsEmpty()) Current.Scope = TEXT("glob");
            Rules.Add(Current);
        }
        Current = FGameDepotRuleConfig();
        bHasCurrent = false;
    };

    for (FString Line : Lines)
    {
        Line = Line.TrimStartAndEnd();
        if (Line.StartsWith(TEXT("- pattern:")))
        {
            FlushCurrent();
            Current.Pattern = StripYamlQuotes(Line.RightChop(FString(TEXT("- pattern:")).Len()));
            bHasCurrent = true;
        }
        else if (bHasCurrent && Line.StartsWith(TEXT("pattern:")))
        {
            Current.Pattern = ReadRuleValue(Line, TEXT("pattern"));
        }
        else if (bHasCurrent && Line.StartsWith(TEXT("mode:")))
        {
            Current.Mode = ReadRuleValue(Line, TEXT("mode"));
        }
        else if (bHasCurrent && Line.StartsWith(TEXT("kind:")))
        {
            Current.Kind = ReadRuleValue(Line, TEXT("kind"));
        }
        else if (bHasCurrent && Line.StartsWith(TEXT("scope:")))
        {
            Current.Scope = ReadRuleValue(Line, TEXT("scope"));
        }
    }
    FlushCurrent();
    return Rules;
}

FString FGameDepotConfigManager::RulesToText(const TArray<FGameDepotRuleConfig>& Rules)
{
    FString Out = TEXT("rules:\n");
    for (const FGameDepotRuleConfig& Rule : Rules)
    {
        Out += FString::Printf(TEXT("  - pattern: %s\n"), *EscapeRuleValue(Rule.Pattern));
        Out += FString::Printf(TEXT("    mode: %s\n"), *Rule.Mode.ToLower());
        Out += FString::Printf(TEXT("    scope: %s\n"), *Rule.Scope);
    }
    return Out;
}

FString FGameDepotConfigManager::ReadScalar(const FString& Content, const FString& Key, const FString& Fallback)
{
    TArray<FString> Lines;
    Content.ParseIntoArrayLines(Lines, false);
    const FString Prefix = Key + TEXT(":");
    for (FString Line : Lines)
    {
        Line = Line.TrimStartAndEnd();
        if (!Line.StartsWith(Prefix))
        {
            continue;
        }
        FString Value = Line.RightChop(Prefix.Len()).TrimStartAndEnd();
        return StripYamlQuotes(Value);
    }
    return Fallback;
}

FString FGameDepotConfigManager::ToConfigFileText(const FGameDepotConfigSnapshot& InSnapshot)
{
    return FString::Printf(
        TEXT("# GameDepot UE config cache. Git remotes are managed by Git.\n")
        TEXT("initialized: true\n")
        TEXT("oss_provider: %s\n")
        TEXT("oss_endpoint: %s\n")
        TEXT("oss_bucket: %s\n")
        TEXT("oss_region: %s\n")
        TEXT("oss_prefix: %s\n")
        TEXT("rules_file: mock-rules.yaml\n"),
        *InSnapshot.OSSProvider,
        *InSnapshot.OSSEndpoint,
        *InSnapshot.OSSBucket,
        *InSnapshot.OSSRegion,
        *InSnapshot.OSSPrefix);
}
