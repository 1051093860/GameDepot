#pragma once

#include "CoreMinimal.h"
#include "Modules/ModuleManager.h"
#include "AssetRegistry/AssetData.h"

DECLARE_LOG_CATEGORY_EXTERN(LogGameDepotUE, Log, All);

class FExtender;
class FMenuBuilder;
class FGameDepotMockStatusProvider;
class SGameDepotStatusPanel;
class FGameDepotConfigManager;
class SGameDepotConfigPanel;
class SGameDepotOperationDialog;
struct FGameDepotConflictItem;
struct FGameDepotHistoryEntry;

enum class EGameDepotToolbarState : uint8
{
    OK,
    Error,
    PendingUpdate
};

class FGameDepotUEModule : public IModuleInterface
{
public:
    virtual void StartupModule() override;
    virtual void ShutdownModule() override;

private:
    TSharedPtr<FGameDepotMockStatusProvider> Provider;
    TWeakPtr<SGameDepotStatusPanel> StatusPanel;
    TSharedPtr<FGameDepotConfigManager> ConfigManager;
    TWeakPtr<SGameDepotConfigPanel> ConfigPanel;
    FDelegateHandle ContentBrowserExtenderDelegateHandle;
    int32 MaxMockAssets = 300;
    bool bMockMode = false;
    bool bAutoStartDaemon = true;
    bool bAutoShutdownDaemon = true;
    bool bStartedDaemon = false;
    FString GameDepotExe;
    FString GameDepotDaemonExe;
    FString DaemonListenAddress = TEXT("127.0.0.1:0");
    FString ProjectRoot;
    FString DaemonAddress;
    FString DaemonToken;
    FProcHandle DaemonProcessHandle;
    TWeakPtr<class SNotificationItem> ActiveTaskNotification;
    TWeakPtr<SGameDepotOperationDialog> OperationDialog;
    EGameDepotToolbarState ToolbarState = EGameDepotToolbarState::PendingUpdate;
    FString ToolbarStateMessage = TEXT("GameDepot is starting.");

    void LoadSettings();
    void RegisterMenus();
    void RegisterStatusTab();
    void RegisterConfigTab();
    void RegisterContentBrowserMenu();
    void UnregisterContentBrowserMenu();

    TSharedRef<class SDockTab> SpawnStatusTab(const class FSpawnTabArgs& Args);
    TSharedRef<class SDockTab> SpawnConfigTab(const class FSpawnTabArgs& Args);
    void OpenStatusTab();
    void OpenConfigTab();
    void RunInitializationCheck();
    void PromptInitializeIfNeeded();
    void InitializeWorkspace();
    void OnConfigSaved();
    void RefreshData();
    void Publish();
    void Update();
    void ApplyRuleToPaths(const TArray<FString>& Paths, const FString& Mode);
    void ShowHistoryForPath(const FString& DepotPath);
    void RestoreHistoryVersion(const FString& DepotPath, const FGameDepotHistoryEntry& Entry);
    void RevertPaths(const TArray<FString>& Paths);
    void OnStatusPanelHistoryRestored(const FString& DepotPath);

    void SetToolbarState(EGameDepotToolbarState NewState, const FString& Message);
    TSharedRef<FExtender> OnExtendContentBrowserAssetSelectionMenu(const TArray<FAssetData>& SelectedAssets);
    void AddContentBrowserMenu(FMenuBuilder& MenuBuilder, TArray<FAssetData> SelectedAssets);
    void ShowSelectedInStatus(TArray<FAssetData> SelectedAssets);
    void SetSelectedRule(TArray<FAssetData> SelectedAssets, FString Mode);
    void ShowSelectedHistory(TArray<FAssetData> SelectedAssets);
    void RevertSelectedUncommittedChanges(TArray<FAssetData> SelectedAssets);
    TArray<FString> AssetDataArrayToDepotPaths(const TArray<FAssetData>& SelectedAssets) const;
    void Notify(const FString& Message) const;

    bool RunGameDepotCLI(const FString& Args, int32& OutCode, FString& OutStdOut, FString& OutStdErr) const;
    bool EnsureDaemon();
    bool ReadRuntimeFile();
    bool LaunchDaemon();
    void ShutdownDaemon();
    FString GetRuntimeFile() const;
    FString GetURL(const FString& Path) const;
    FString GetAPILogFile() const;
    void LogJSON(const FString& Title, const FString& Text) const;
    void RequestJSON(const FString& Method, const FString& Path, const FString& Body, TFunction<void(bool, const FString&)> Callback);
    void RequestJSONInternal(const FString& Method, const FString& Path, const FString& Body, TFunction<void(bool, const FString&)> Callback, bool bRetryOnNoResponse);
    FString PathsToJSONBody(const TArray<FString>& Paths, const FString& ExtraFields = FString()) const;
    void StartTaskEndpoint(const FString& Path, const FString& Body, const FString& FriendlyName);
    void PollTask(const FString& TaskID, const FString& FriendlyName);
    void StartGameDepotOperation(const FString& FriendlyName, const FString& Path, const FString& Body);
    void PollGameDepotOperation(const FString& TaskID, const FString& FriendlyName);
    void ShowConflictDialog();
    void ShowConflictDialogFromJSON(const FString& JsonText);
    void ResolveConflict(const FString& DepotPath, const FString& Decision);
    bool PromptPublishMessage(FString& OutMessage) const;
    bool SaveDirtyContentPackages(const FString& Reason) const;
    void RefreshContentBrowserAfterDepotChange();
    void PullConfigFromDaemon(bool bRefreshPanel);
    void PushConfigToDaemon(const struct FGameDepotConfigSnapshot& Snapshot);
    void RefreshOverview();
    void ParseAndApplyOverview(const FString& JsonText);
};
