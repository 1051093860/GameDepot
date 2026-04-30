#pragma once

#include "CoreMinimal.h"

struct FGameDepotRuleConfig
{
    FString Pattern;
    FString Mode;
    FString Kind;
    FString Scope;
};

struct FGameDepotConfigSnapshot
{
    bool bInitialized = false;
    FString GitRemoteURL;
    FString GitBranch;
    FString OSSProvider;
    FString OSSEndpoint;
    FString OSSBucket;
    FString OSSRegion;
    FString OSSPrefix;
    TArray<FGameDepotRuleConfig> Rules;
    FString RuleText;
};

class FGameDepotConfigManager : public TSharedFromThis<FGameDepotConfigManager>
{
public:
    FGameDepotConfigManager();

    void Load();
    bool Save(const FGameDepotConfigSnapshot& NewSnapshot, FString& OutError);
    bool InitializeDefault(FString& OutError);

    bool IsInitialized() const { return Snapshot.bInitialized; }
    const FGameDepotConfigSnapshot& GetSnapshot() const { return Snapshot; }
    void SetSnapshot(const FGameDepotConfigSnapshot& NewSnapshot) { Snapshot = NewSnapshot; }

    FString GetProjectRoot() const { return ProjectRoot; }
    FString GetDepotDir() const { return DepotDir; }
    FString GetConfigPath() const { return ConfigPath; }
    FString GetRulesPath() const { return RulesPath; }

    FText BuildValidationSummary() const;

    static FString RulesToText(const TArray<FGameDepotRuleConfig>& Rules);
    static TArray<FGameDepotRuleConfig> ParseRules(const FString& RuleContent);

private:
    FString ProjectRoot;
    FString DepotDir;
    FString ConfigPath;
    FString RulesPath;
    FGameDepotConfigSnapshot Snapshot;

    static FGameDepotConfigSnapshot MakeDefaultSnapshot();
    static TArray<FGameDepotRuleConfig> DefaultRules();
    static FString ReadScalar(const FString& Content, const FString& Key, const FString& Fallback);
    static FString ToConfigFileText(const FGameDepotConfigSnapshot& InSnapshot);
};
