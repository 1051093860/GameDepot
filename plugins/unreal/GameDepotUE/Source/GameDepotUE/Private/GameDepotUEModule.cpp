#include "GameDepotUEModule.h"

#include "ContentBrowserModule.h"
#include "ContentBrowserDelegates.h"
#include "Containers/Ticker.h"
#include "Dom/JsonObject.h"
#include "Framework/Application/SlateApplication.h"
#include "Framework/Docking/TabManager.h"
#include "Framework/MultiBox/MultiBoxBuilder.h"
#include "Framework/Notifications/NotificationManager.h"
#include "GameDepotConfigManager.h"
#include "GameDepotMockStatusProvider.h"
#include "HttpModule.h"
#include "IContentBrowserSingleton.h"
#include "Interfaces/IHttpRequest.h"
#include "Interfaces/IHttpResponse.h"
#include "HAL/FileManager.h"
#include "Misc/App.h"
#include "Misc/ConfigCacheIni.h"
#include "Misc/FileHelper.h"
#include "Misc/MessageDialog.h"
#include "Misc/Paths.h"
#include "SGameDepotConfigPanel.h"
#include "SGameDepotHistoryDialog.h"
#include "SGameDepotStatusPanel.h"
#include "Serialization/JsonReader.h"
#include "Serialization/JsonSerializer.h"
#include "Styling/AppStyle.h"
#include "ToolMenus.h"
#include "Widgets/Docking/SDockTab.h"
#include "Widgets/Notifications/SNotificationList.h"
#include "Widgets/SWindow.h"
#include "Widgets/Text/STextBlock.h"

DEFINE_LOG_CATEGORY(LogGameDepotUE);
#define LOCTEXT_NAMESPACE "FGameDepotUEModule"

namespace
{
static const FName GameDepotStatusTabName(TEXT("GameDepotStatus"));
static const FName GameDepotConfigTabName(TEXT("GameDepotConfig"));

FString JsonEscape(const FString& In)
{
    FString Out = In;
    Out.ReplaceInline(TEXT("\\"), TEXT("\\\\"));
    Out.ReplaceInline(TEXT("\""), TEXT("\\\""));
    Out.ReplaceInline(TEXT("\r"), TEXT("\\r"));
    Out.ReplaceInline(TEXT("\n"), TEXT("\\n"));
    return Out;
}

TSharedRef<SWidget> MakeGameDepotMenuLabel(const FText& Text)
{
    return SNew(STextBlock).Text(Text).ToolTipText(Text);
}

bool ParseJsonObject(const FString& Text, TSharedPtr<FJsonObject>& Out)
{
    const TSharedRef<TJsonReader<>> Reader = TJsonReaderFactory<>::Create(Text);
    return FJsonSerializer::Deserialize(Reader, Out) && Out.IsValid();
}

bool TryGetStringField(const FString& JsonText, const FString& Field, FString& Out)
{
    TSharedPtr<FJsonObject> Obj;
    if (ParseJsonObject(JsonText, Obj))
    {
        return Obj->TryGetStringField(Field, Out);
    }
    return false;
}

bool TryGetObjectField(const FString& JsonText, const FString& Field, TSharedPtr<FJsonObject>& Out)
{
    TSharedPtr<FJsonObject> Obj;
    if (ParseJsonObject(JsonText, Obj))
    {
        const TSharedPtr<FJsonObject>* Found = nullptr;
        if (Obj->TryGetObjectField(Field, Found) && Found && Found->IsValid())
        {
            Out = *Found;
            return true;
        }
    }
    return false;
}

FString QuoteJSON(const FString& S)
{
    return FString::Printf(TEXT("\"%s\""), *JsonEscape(S));
}
}

void FGameDepotUEModule::StartupModule()
{
    ProjectRoot = FPaths::ConvertRelativePathToFull(FPaths::ProjectDir());
    LoadSettings();

    ConfigManager = MakeShared<FGameDepotConfigManager>();
    ConfigManager->Load();
    Provider = MakeShared<FGameDepotMockStatusProvider>();

    RegisterStatusTab();
    RegisterConfigTab();
    RegisterContentBrowserMenu();
    UToolMenus::RegisterStartupCallback(FSimpleMulticastDelegate::FDelegate::CreateRaw(this, &FGameDepotUEModule::RegisterMenus));

    RunInitializationCheck();
    UE_LOG(LogGameDepotUE, Display, TEXT("GameDepot UE loaded. MockMode=%s Project=%s"), bMockMode ? TEXT("true") : TEXT("false"), *ProjectRoot);
}

void FGameDepotUEModule::ShutdownModule()
{
    UnregisterContentBrowserMenu();
    FGlobalTabmanager::Get()->UnregisterNomadTabSpawner(GameDepotStatusTabName);
    FGlobalTabmanager::Get()->UnregisterNomadTabSpawner(GameDepotConfigTabName);
    if (UToolMenus::IsToolMenuUIEnabled())
    {
        UToolMenus::UnRegisterStartupCallback(this);
        UToolMenus::UnregisterOwner(this);
    }
    ShutdownDaemon();
}

void FGameDepotUEModule::LoadSettings()
{
    if (GConfig)
    {
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("MockMode"), bMockMode, GGameIni);
        GConfig->GetInt(TEXT("GameDepotUE"), TEXT("MaxMockAssets"), MaxMockAssets, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("GameDepotExecutable"), GameDepotExe, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("GameDepotDaemonExecutable"), GameDepotDaemonExe, GGameIni);
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("AutoStartDaemon"), bAutoStartDaemon, GGameIni);
        GConfig->GetBool(TEXT("GameDepotUE"), TEXT("AutoShutdownDaemon"), bAutoShutdownDaemon, GGameIni);
        GConfig->GetString(TEXT("GameDepotUE"), TEXT("DaemonListenAddress"), DaemonListenAddress, GGameIni);
    }
    MaxMockAssets = FMath::Clamp(MaxMockAssets, 20, 5000);
    if (DaemonListenAddress.IsEmpty()) DaemonListenAddress = TEXT("127.0.0.1:0");

    TArray<FString> Candidates;
    if (!GameDepotExe.IsEmpty()) Candidates.Add(GameDepotExe);
    Candidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT(".."), TEXT("GameDepot"), TEXT("gamedepot.exe"))));
    Candidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT(".."), TEXT("gamedepot.exe"))));
    Candidates.Add(FPaths::ConvertRelativePathToFull(FPaths::Combine(ProjectRoot, TEXT("gamedepot.exe"))));
    for (const FString& Candidate : Candidates)
    {
        if (FPaths::FileExists(Candidate))
        {
            GameDepotExe = Candidate;
            break;
        }
    }
    if (GameDepotDaemonExe.IsEmpty()) GameDepotDaemonExe = GameDepotExe;
}

void FGameDepotUEModule::RegisterMenus()
{
    FToolMenuOwnerScoped OwnerScoped(this);
    if (UToolMenu* Menu = UToolMenus::Get()->ExtendMenu("LevelEditor.MainMenu.Tools"))
    {
        FToolMenuSection& Section = Menu->FindOrAddSection("GameDepot");
        Section.Label = LOCTEXT("GameDepotSection", "GameDepot");
        Section.AddMenuEntry("GameDepotOpenStatus", LOCTEXT("OpenStatus", "Asset Status Browser"), LOCTEXT("OpenStatusTooltip", "Open GameDepot asset status UI."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "LevelEditor.Tabs.ContentBrowser"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab)));
        Section.AddMenuEntry("GameDepotOpenConfig", LOCTEXT("OpenConfig", "Configuration Manager"), LOCTEXT("OpenConfigTooltip", "Configure OSS/blob storage and rules."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Settings"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenConfigTab)));
        Section.AddMenuEntry("GameDepotInitCheck", LOCTEXT("InitCheck", "Run Initialization Check"), LOCTEXT("InitCheckTooltip", "Check or initialize GameDepot for this project."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Warning"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::PromptInitializeIfNeeded)));
        Section.AddMenuEntry("GameDepotRefresh", LOCTEXT("RefreshStatus", "Refresh Status"), LOCTEXT("RefreshStatusTooltip", "Refresh GameDepot status."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Refresh"), FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::RefreshData)));
    }

    if (UToolMenu* Toolbar = UToolMenus::Get()->ExtendMenu("LevelEditor.LevelEditorToolBar.User"))
    {
        FToolMenuSection& Section = Toolbar->FindOrAddSection("GameDepot");
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotOpenStatusToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab)), LOCTEXT("ToolbarStatus", "Status"), LOCTEXT("ToolbarStatusTooltip", "Open GameDepot asset status browser."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "LevelEditor.Tabs.ContentBrowser")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotOpenConfigToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenConfigTab)), LOCTEXT("ToolbarConfig", "Config"), LOCTEXT("ToolbarConfigTooltip", "Open GameDepot configuration manager."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Settings")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotSyncToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::Sync)), LOCTEXT("ToolbarSync", "Sync"), LOCTEXT("ToolbarSyncTooltip", "Run GameDepot sync."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Refresh")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotSubmitToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::Submit)), LOCTEXT("ToolbarSubmit", "Submit"), LOCTEXT("ToolbarSubmitTooltip", "Submit Git/OSS changes through GameDepot."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Upload")));
        Section.AddSeparator("GameDepotToolbarStateSeparator");
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotStateOKToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab), FCanExecuteAction::CreateRaw(this, &FGameDepotUEModule::IsToolbarStateOK)), LOCTEXT("ToolbarStateOK", "OK"), LOCTEXT("ToolbarStateOKTooltip", "Last operation completed successfully."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Check")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotStateErrorToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab), FCanExecuteAction::CreateRaw(this, &FGameDepotUEModule::IsToolbarStateError)), LOCTEXT("ToolbarStateError", "Error"), LOCTEXT("ToolbarStateErrorTooltip", "Last operation had errors."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Error")));
        Section.AddEntry(FToolMenuEntry::InitToolBarButton("GameDepotStatePendingToolbar", FUIAction(FExecuteAction::CreateRaw(this, &FGameDepotUEModule::OpenStatusTab), FCanExecuteAction::CreateRaw(this, &FGameDepotUEModule::IsToolbarStatePendingUpdate)), LOCTEXT("ToolbarStatePending", "Update"), LOCTEXT("ToolbarStatePendingTooltip", "Pending sync or submit."), FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Warning")));
    }
}

void FGameDepotUEModule::RegisterStatusTab()
{
    FGlobalTabmanager::Get()->RegisterNomadTabSpawner(GameDepotStatusTabName, FOnSpawnTab::CreateRaw(this, &FGameDepotUEModule::SpawnStatusTab))
        .SetDisplayName(LOCTEXT("StatusTabTitle", "GameDepot Status"))
        .SetTooltipText(LOCTEXT("StatusTabTooltip", "Git/OSS routing and sync status."))
        .SetIcon(FSlateIcon(FAppStyle::GetAppStyleSetName(), "LevelEditor.Tabs.ContentBrowser"));
}

void FGameDepotUEModule::RegisterConfigTab()
{
    FGlobalTabmanager::Get()->RegisterNomadTabSpawner(GameDepotConfigTabName, FOnSpawnTab::CreateRaw(this, &FGameDepotUEModule::SpawnConfigTab))
        .SetDisplayName(LOCTEXT("ConfigTabTitle", "GameDepot Config"))
        .SetTooltipText(LOCTEXT("ConfigTabTooltip", "Configuration manager for OSS/blob storage and rules."))
        .SetIcon(FSlateIcon(FAppStyle::GetAppStyleSetName(), "Icons.Settings"));
}

void FGameDepotUEModule::RegisterContentBrowserMenu()
{
    FContentBrowserModule& ContentBrowserModule = FModuleManager::LoadModuleChecked<FContentBrowserModule>(TEXT("ContentBrowser"));
    TArray<FContentBrowserMenuExtender_SelectedAssets>& Extenders = ContentBrowserModule.GetAllAssetViewContextMenuExtenders();
    Extenders.Add(FContentBrowserMenuExtender_SelectedAssets::CreateRaw(this, &FGameDepotUEModule::OnExtendContentBrowserAssetSelectionMenu));
    ContentBrowserExtenderDelegateHandle = Extenders.Last().GetHandle();
}

void FGameDepotUEModule::UnregisterContentBrowserMenu()
{
    if (!ContentBrowserExtenderDelegateHandle.IsValid() || !FModuleManager::Get().IsModuleLoaded(TEXT("ContentBrowser"))) return;
    FContentBrowserModule& ContentBrowserModule = FModuleManager::GetModuleChecked<FContentBrowserModule>(TEXT("ContentBrowser"));
    TArray<FContentBrowserMenuExtender_SelectedAssets>& Extenders = ContentBrowserModule.GetAllAssetViewContextMenuExtenders();
    Extenders.RemoveAll([this](const FContentBrowserMenuExtender_SelectedAssets& Delegate) { return Delegate.GetHandle() == ContentBrowserExtenderDelegateHandle; });
    ContentBrowserExtenderDelegateHandle.Reset();
}

TSharedRef<SDockTab> FGameDepotUEModule::SpawnStatusTab(const FSpawnTabArgs& Args)
{
    if (!Provider.IsValid()) Provider = MakeShared<FGameDepotMockStatusProvider>();
    if (Provider->GetRows().Num() == 0) RefreshData();
    TSharedRef<SGameDepotStatusPanel> Panel = SNew(SGameDepotStatusPanel)
        .Provider(Provider)
        .OnRefreshRequested(FSimpleDelegate::CreateRaw(this, &FGameDepotUEModule::RefreshData))
        .OnRuleRequested(FGameDepotRulePathsAction::CreateRaw(this, &FGameDepotUEModule::ApplyRuleToPaths))
        .OnHistoryRequested(FGameDepotPathAction::CreateRaw(this, &FGameDepotUEModule::ShowHistoryForPath))
        .OnRevertRequested(FGameDepotPathsAction::CreateRaw(this, &FGameDepotUEModule::RevertPaths))
        .OnHistoryRestored(FGameDepotPathAction::CreateRaw(this, &FGameDepotUEModule::OnStatusPanelHistoryRestored));
    StatusPanel = Panel;
    return SNew(SDockTab).TabRole(ETabRole::NomadTab)[Panel];
}

TSharedRef<SDockTab> FGameDepotUEModule::SpawnConfigTab(const FSpawnTabArgs& Args)
{
    if (!ConfigManager.IsValid()) ConfigManager = MakeShared<FGameDepotConfigManager>();
    TSharedRef<SGameDepotConfigPanel> Panel = SNew(SGameDepotConfigPanel).ConfigManager(ConfigManager).OnConfigSaved(FSimpleDelegate::CreateRaw(this, &FGameDepotUEModule::OnConfigSaved));
    ConfigPanel = Panel;
    if (!bMockMode) PullConfigFromDaemon(true);
    return SNew(SDockTab).TabRole(ETabRole::NomadTab)[Panel];
}

void FGameDepotUEModule::OpenStatusTab() { FGlobalTabmanager::Get()->TryInvokeTab(GameDepotStatusTabName); }
void FGameDepotUEModule::OpenConfigTab() { FGlobalTabmanager::Get()->TryInvokeTab(GameDepotConfigTabName); }

void FGameDepotUEModule::RunInitializationCheck()
{
    if (bMockMode)
    {
        ConfigManager->Load();
        if (!ConfigManager->IsInitialized()) PromptInitializeIfNeeded();
        else { Provider->RebuildFromAssetRegistry(MaxMockAssets); SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock config loaded.")); }
        return;
    }
    const bool bConfigExists = FPaths::FileExists(FPaths::Combine(ProjectRoot, TEXT(".gamedepot/config.yaml")));
    if (!bConfigExists)
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("GameDepot is not initialized."));
        PromptInitializeIfNeeded();
        return;
    }
    if (EnsureDaemon())
    {
        PullConfigFromDaemon(false);
        RefreshOverview();
        RefreshData();
    }
}

void FGameDepotUEModule::PromptInitializeIfNeeded()
{
    if (bMockMode)
    {
        if (ConfigManager.IsValid() && ConfigManager->IsInitialized()) { OpenConfigTab(); return; }
        const EAppReturnType::Type Result = FMessageDialog::Open(EAppMsgType::YesNoCancel, LOCTEXT("InitDialogTextMock", "GameDepot mock UI is not initialized for this project.\n\nYes: create mock config.\nNo: open Configuration Manager.\nCancel: skip."));
        if (Result == EAppReturnType::Yes) InitializeWorkspace();
        else if (Result == EAppReturnType::No) OpenConfigTab();
        return;
    }
    if (FPaths::FileExists(FPaths::Combine(ProjectRoot, TEXT(".gamedepot/config.yaml")))) { OpenConfigTab(); return; }
    const EAppReturnType::Type Result = FMessageDialog::Open(EAppMsgType::YesNoCancel, LOCTEXT("InitDialogText", "GameDepot is not initialized for this project.\n\nYes: initialize now.\nNo: open Configuration Manager.\nCancel: skip for this session."));
    if (Result == EAppReturnType::Yes) InitializeWorkspace();
    else if (Result == EAppReturnType::No) OpenConfigTab();
}

void FGameDepotUEModule::InitializeWorkspace()
{
    if (bMockMode)
    {
        FString Error;
        if (ConfigManager->InitializeDefault(Error)) { SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock workspace initialized.")); Notify(TEXT("GameDepot mock workspace initialized.")); OpenConfigTab(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); }
        return;
    }
    if (GameDepotExe.IsEmpty() || !FPaths::FileExists(GameDepotExe))
    {
        SetToolbarState(EGameDepotToolbarState::Error, TEXT("gamedepot.exe not found. Set GameDepotExecutable in DefaultGameDepotUE.ini."));
        Notify(ToolbarStateMessage);
        return;
    }
    int32 Code = -1; FString Out, Err;
    const FString ProjectName = FPaths::GetBaseFilename(FPaths::GetProjectFilePath()).IsEmpty() ? FPaths::GetBaseFilename(ProjectRoot) : FPaths::GetBaseFilename(FPaths::GetProjectFilePath());
    const FString Args = FString::Printf(TEXT("init --project \"%s\" --template ue5"), *ProjectName);
    if (RunGameDepotCLI(Args, Code, Out, Err))
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("GameDepot initialized. Starting daemon..."));
        Notify(TEXT("GameDepot initialized."));
        EnsureDaemon();
        PullConfigFromDaemon(true);
        RefreshData();
    }
    else
    {
        SetToolbarState(EGameDepotToolbarState::Error, Err.IsEmpty() ? Out : Err);
        Notify(ToolbarStateMessage);
    }
}

void FGameDepotUEModule::OnConfigSaved()
{
    if (!ConfigManager.IsValid()) return;
    ConfigManager->Load();
    if (bMockMode)
    {
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock config changed. Sync or submit is pending."));
        Notify(TEXT("GameDepot mock config saved."));
        return;
    }
    PushConfigToDaemon(ConfigManager->GetSnapshot());
}

void FGameDepotUEModule::RefreshData()
{
    if (!Provider.IsValid()) Provider = MakeShared<FGameDepotMockStatusProvider>();
    if (bMockMode)
    {
        Provider->RebuildFromAssetRegistry(MaxMockAssets);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock status refreshed."));
        return;
    }
    if (!EnsureDaemon()) return;
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/status"), TEXT("{\"paths\":[],\"include_history\":true,\"include_remote\":false}"), [this](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Refresh status failed.")); return; }
        FString Error;
        if (!Provider->ReplaceRowsFromDaemonJSON(Text, Error)) { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); return; }
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        RefreshOverview();
    });
}

void FGameDepotUEModule::Submit()
{
    if (bMockMode)
    {
        const int32 ErrorCount = Provider.IsValid() ? Provider->CountBySeverity(EGameDepotSeverity::Error) : 0;
        const int32 ReviewCount = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::ReviewRequired) : 0;
        SetToolbarState((ErrorCount || ReviewCount) ? EGameDepotToolbarState::Error : EGameDepotToolbarState::OK, (ErrorCount || ReviewCount) ? FString::Printf(TEXT("Mock submit blocked: %d errors, %d review."), ErrorCount, ReviewCount) : TEXT("Mock submit succeeded."));
        Notify(ToolbarStateMessage);
        return;
    }
    if (!EnsureDaemon()) return;
    const EAppReturnType::Type Choice = FMessageDialog::Open(EAppMsgType::YesNo, LOCTEXT("SubmitConfirm", "Submit all saved changes through GameDepot?"));
    if (Choice != EAppReturnType::Yes) return;
    StartTaskEndpoint(TEXT("/api/ue/v1/submit"), TEXT("{\"message\":\"Submit from Unreal Editor\",\"push\":false,\"verify_after_submit\":false}"), TEXT("Submit"));
}

void FGameDepotUEModule::Sync()
{
    if (bMockMode)
    {
        SetToolbarState(EGameDepotToolbarState::OK, TEXT("Mock sync succeeded."));
        Notify(ToolbarStateMessage);
        return;
    }
    if (!EnsureDaemon()) return;
    StartTaskEndpoint(TEXT("/api/ue/v1/sync"), TEXT("{\"force\":false}"), TEXT("Sync"));
}

void FGameDepotUEModule::ApplyRuleToPaths(const TArray<FString>& Paths, const FString& Mode)
{
    if (Paths.Num() == 0) return;
    if (bMockMode)
    {
        Provider->SetRuleForDepotPaths(Paths, Mode);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Mock rule changed."));
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Kind = Mode == TEXT("git") ? TEXT("manual_git") : (Mode == TEXT("ignore") ? TEXT("manual_ignore") : TEXT("manual_blob"));
    const FString Body = PathsToJSONBody(Paths, FString::Printf(TEXT("\"mode\":%s,\"kind\":%s,\"scope\":\"exact\""), *QuoteJSON(Mode), *QuoteJSON(Kind)));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/rules/upsert"), Body, [this, Mode](bool bOK, const FString& Text)
    {
        if (bOK) { SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("Rule changed to %s. Submit is pending."), *Mode)); RefreshData(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Set rule failed.")); }
    });
}

void FGameDepotUEModule::ShowHistoryForPath(const FString& DepotPath)
{
    if (DepotPath.IsEmpty()) return;
    if (bMockMode)
    {
        TSharedRef<SWindow> Window = SNew(SWindow).Title(FText::FromString(FString::Printf(TEXT("GameDepot History - %s"), *DepotPath))).ClientSize(FVector2D(860, 520)).SupportsMaximize(false).SupportsMinimize(false);
        Window->SetContent(SNew(SGameDepotHistoryDialog).DepotPath(DepotPath).Provider(Provider).ParentWindow(TWeakPtr<SWindow>(Window)).OnRestored(FSimpleDelegate::CreateLambda([this, DepotPath]() { OnStatusPanelHistoryRestored(DepotPath); })));
        FSlateApplication::Get().AddWindow(Window);
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"path\":%s}"), *QuoteJSON(DepotPath));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/history"), Body, [this, DepotPath](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("History query failed.")); return; }
        FString Error;
        if (!Provider->SetHistoryFromDaemonJSON(DepotPath, Text, Error)) { SetToolbarState(EGameDepotToolbarState::Error, Error); Notify(Error); return; }
        TSharedRef<SWindow> Window = SNew(SWindow).Title(FText::FromString(FString::Printf(TEXT("GameDepot History - %s"), *DepotPath))).ClientSize(FVector2D(860, 520)).SupportsMaximize(false).SupportsMinimize(false);
        Window->SetContent(SNew(SGameDepotHistoryDialog).DepotPath(DepotPath).Provider(Provider).ParentWindow(TWeakPtr<SWindow>(Window)).OnRestoreVersion(FGameDepotRestoreVersionAction::CreateRaw(this, &FGameDepotUEModule::RestoreHistoryVersion)).OnRestored(FSimpleDelegate::CreateLambda([this, DepotPath]() { OnStatusPanelHistoryRestored(DepotPath); })));
        FSlateApplication::Get().AddWindow(Window);
    });
}

void FGameDepotUEModule::RestoreHistoryVersion(const FString& DepotPath, const FGameDepotHistoryEntry& Entry)
{
    if (bMockMode)
    {
        Provider->RestoreDepotPathToHistory(DepotPath, Entry);
        OnStatusPanelHistoryRestored(DepotPath);
        return;
    }
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"path\":%s,\"commit\":%s,\"force\":true}"), *QuoteJSON(DepotPath), *QuoteJSON(Entry.CommitId));
    StartTaskEndpoint(TEXT("/api/ue/v1/assets/restore"), Body, TEXT("Restore history version"));
}

void FGameDepotUEModule::RevertPaths(const TArray<FString>& Paths)
{
    if (Paths.Num() == 0) return;
    if (bMockMode)
    {
        Provider->RevertUncommittedChanges(Paths);
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) Panel->RefreshRows();
        SetToolbarState(EGameDepotToolbarState::OK, TEXT("Mock revert completed."));
        return;
    }
    if (!EnsureDaemon()) return;
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/assets/revert"), PathsToJSONBody(Paths, TEXT("\"force\":true")), [this](bool bOK, const FString& Text)
    {
        if (bOK) { SetToolbarState(EGameDepotToolbarState::OK, TEXT("Revert completed.")); RefreshData(); }
        else { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Revert failed.")); }
    });
}

void FGameDepotUEModule::OnStatusPanelHistoryRestored(const FString& DepotPath)
{
    SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("History version restored for %s. Submit is pending."), *DepotPath));
    RefreshData();
}

void FGameDepotUEModule::SetToolbarState(EGameDepotToolbarState NewState, const FString& Message)
{
    ToolbarState = NewState;
    ToolbarStateMessage = Message;
    if (UToolMenus::IsToolMenuUIEnabled()) UToolMenus::Get()->RefreshAllWidgets();
}

bool FGameDepotUEModule::IsToolbarStateOK() const { return ToolbarState == EGameDepotToolbarState::OK; }
bool FGameDepotUEModule::IsToolbarStateError() const { return ToolbarState == EGameDepotToolbarState::Error; }
bool FGameDepotUEModule::IsToolbarStatePendingUpdate() const { return ToolbarState == EGameDepotToolbarState::PendingUpdate; }

TSharedRef<FExtender> FGameDepotUEModule::OnExtendContentBrowserAssetSelectionMenu(const TArray<FAssetData>& SelectedAssets)
{
    TSharedRef<FExtender> Extender = MakeShared<FExtender>();
    if (SelectedAssets.Num() > 0)
    {
        Extender->AddMenuExtension("GetAssetActions", EExtensionHook::After, nullptr, FMenuExtensionDelegate::CreateRaw(this, &FGameDepotUEModule::AddContentBrowserMenu, SelectedAssets));
    }
    return Extender;
}

void FGameDepotUEModule::AddContentBrowserMenu(FMenuBuilder& MenuBuilder, TArray<FAssetData> SelectedAssets)
{
    MenuBuilder.BeginSection("GameDepot", LOCTEXT("GameDepotMenuHeading", "GameDepot"));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { ShowSelectedInStatus(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("ShowInStatus", "Show in GameDepot Status")), NAME_None, LOCTEXT("ShowInStatusTooltip", "Open GameDepot status tab."));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { ShowSelectedHistory(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("ShowHistory", "Show History Versions...")), NAME_None, LOCTEXT("ShowHistoryTooltip", "Show Git/OSS history versions."));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { RevertSelectedUncommittedChanges(SelectedAssets); })), MakeGameDepotMenuLabel(LOCTEXT("RevertUncommitted", "Revert Unsubmitted Changes")), NAME_None, LOCTEXT("RevertUncommittedTooltip", "Discard local unsubmitted changes."));
    MenuBuilder.AddMenuSeparator();
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { SetSelectedRule(SelectedAssets, TEXT("blob")); })), MakeGameDepotMenuLabel(LOCTEXT("SetRuleOSS", "Set Rule: OSS / Blob")), NAME_None, LOCTEXT("SetRuleOSSTooltip", "Route selected assets through OSS/blob storage."));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { SetSelectedRule(SelectedAssets, TEXT("git")); })), MakeGameDepotMenuLabel(LOCTEXT("SetRuleGit", "Set Rule: Git")), NAME_None, LOCTEXT("SetRuleGitTooltip", "Route selected assets directly through Git."));
    MenuBuilder.AddMenuEntry(FUIAction(FExecuteAction::CreateLambda([this, SelectedAssets]() { SetSelectedRule(SelectedAssets, TEXT("ignore")); })), MakeGameDepotMenuLabel(LOCTEXT("SetRuleIgnore", "Set Rule: Ignore")), NAME_None, LOCTEXT("SetRuleIgnoreTooltip", "Ignore selected assets."));
    MenuBuilder.EndSection();
}

void FGameDepotUEModule::ShowSelectedInStatus(TArray<FAssetData> SelectedAssets)
{
    const TArray<FString> Paths = AssetDataArrayToDepotPaths(SelectedAssets);
    if (bMockMode)
    {
        Provider->ClearSelectionMarks(); Provider->AddOrUpdateAssets(SelectedAssets, true); OpenStatusTab();
        if (TSharedPtr<SGameDepotStatusPanel> Panel = StatusPanel.Pin()) { Panel->RefreshRows(); if (Paths.Num() > 0) Panel->FocusPath(Paths[0]); }
        return;
    }
    OpenStatusTab();
    RefreshData();
}

void FGameDepotUEModule::SetSelectedRule(TArray<FAssetData> SelectedAssets, FString Mode) { ApplyRuleToPaths(AssetDataArrayToDepotPaths(SelectedAssets), Mode); }
void FGameDepotUEModule::ShowSelectedHistory(TArray<FAssetData> SelectedAssets) { const TArray<FString> Paths = AssetDataArrayToDepotPaths(SelectedAssets); if (Paths.Num() > 0) ShowHistoryForPath(Paths[0]); }
void FGameDepotUEModule::RevertSelectedUncommittedChanges(TArray<FAssetData> SelectedAssets) { RevertPaths(AssetDataArrayToDepotPaths(SelectedAssets)); }

TArray<FString> FGameDepotUEModule::AssetDataArrayToDepotPaths(const TArray<FAssetData>& SelectedAssets) const
{
    TArray<FString> Paths;
    for (const FAssetData& Asset : SelectedAssets)
    {
        Paths.AddUnique(FGameDepotMockStatusProvider::AssetDataToDepotPath(Asset));
    }
    return Paths;
}

void FGameDepotUEModule::Notify(const FString& Message) const
{
    FNotificationInfo Info(FText::FromString(Message));
    Info.ExpireDuration = 3.0f;
    Info.bUseLargeFont = false;
    FSlateNotificationManager::Get().AddNotification(Info);
    UE_LOG(LogGameDepotUE, Display, TEXT("%s"), *Message);
}

bool FGameDepotUEModule::RunGameDepotCLI(const FString& Args, int32& OutCode, FString& OutStdOut, FString& OutStdErr) const
{
    OutCode = -1; OutStdOut.Reset(); OutStdErr.Reset();
    if (GameDepotExe.IsEmpty() || !FPaths::FileExists(GameDepotExe))
    {
        OutStdErr = TEXT("GameDepotExecutable is missing: ") + GameDepotExe;
        return false;
    }
    const bool bOK = FPlatformProcess::ExecProcess(*GameDepotExe, *Args, &OutCode, &OutStdOut, &OutStdErr);
    return bOK && OutCode == 0;
}

bool FGameDepotUEModule::EnsureDaemon()
{
    if (bMockMode) return true;
    if (ReadRuntimeFile()) return true;
    return bAutoStartDaemon && LaunchDaemon();
}

bool FGameDepotUEModule::ReadRuntimeFile()
{
    FString Text;
    if (!FFileHelper::LoadFileToString(Text, *GetRuntimeFile())) return false;
    TSharedPtr<FJsonObject> Obj;
    if (!ParseJsonObject(Text, Obj)) return false;
    Obj->TryGetStringField(TEXT("addr"), DaemonAddress);
    Obj->TryGetStringField(TEXT("token"), DaemonToken);
    return !DaemonAddress.IsEmpty();
}

bool FGameDepotUEModule::LaunchDaemon()
{
    IFileManager::Get().Delete(*GetRuntimeFile(), false, true, true);
    const FString Exe = FPaths::FileExists(GameDepotDaemonExe) ? GameDepotDaemonExe : GameDepotExe;
    if (Exe.IsEmpty() || !FPaths::FileExists(Exe))
    {
        SetToolbarState(EGameDepotToolbarState::Error, TEXT("gamedepot.exe not found. Set GameDepotExecutable in DefaultGameDepotUE.ini."));
        Notify(ToolbarStateMessage);
        return false;
    }
    const FString Args = FString::Printf(TEXT("daemon --root \"%s\" --addr %s --started-by UnrealEditor"), *ProjectRoot, *DaemonListenAddress);
    uint32 PID = 0;
    DaemonProcessHandle = FPlatformProcess::CreateProc(*Exe, *Args, true, true, true, &PID, 0, nullptr, nullptr);
    bStartedDaemon = DaemonProcessHandle.IsValid();
    for (int32 i = 0; i < 50; ++i)
    {
        FPlatformProcess::Sleep(0.1f);
        if (ReadRuntimeFile()) { Notify(TEXT("GameDepot daemon connected.")); return true; }
    }
    SetToolbarState(EGameDepotToolbarState::Error, TEXT("GameDepot daemon startup timed out."));
    Notify(ToolbarStateMessage);
    return false;
}

void FGameDepotUEModule::ShutdownDaemon()
{
    if (!bMockMode && bAutoShutdownDaemon && !DaemonAddress.IsEmpty())
    {
        RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/admin/shutdown"), TEXT("{}"), [](bool, const FString&) {});
    }
    if (bStartedDaemon && DaemonProcessHandle.IsValid()) FPlatformProcess::CloseProc(DaemonProcessHandle);
}

FString FGameDepotUEModule::GetRuntimeFile() const { return FPaths::Combine(ProjectRoot, TEXT(".gamedepot/runtime/daemon.json")); }
FString FGameDepotUEModule::GetURL(const FString& Path) const { return FString::Printf(TEXT("http://%s%s"), *DaemonAddress, *Path); }
FString FGameDepotUEModule::GetAPILogFile() const { return FPaths::ProjectSavedDir() / TEXT("Logs/GameDepotUE_API.log"); }

void FGameDepotUEModule::LogJSON(const FString& Title, const FString& Text) const
{
    const FString LogFile = GetAPILogFile();
    IFileManager::Get().MakeDirectory(*FPaths::GetPath(LogFile), true);
    const FString Line = FString::Printf(TEXT("\n[%s] %s\n%s\n"), *FDateTime::Now().ToString(), *Title, *Text);
    FFileHelper::SaveStringToFile(Line, *LogFile, FFileHelper::EEncodingOptions::AutoDetect, &IFileManager::Get(), FILEWRITE_Append);
    UE_LOG(LogGameDepotUE, Display, TEXT("%s: %s"), *Title, *Text);
}

void FGameDepotUEModule::RequestJSON(const FString& Method, const FString& Path, const FString& Body, TFunction<void(bool, const FString&)> Callback)
{
    if (DaemonAddress.IsEmpty() && !EnsureDaemon())
    {
        Callback(false, TEXT("daemon_not_ready")); return;
    }
    const FString URL = GetURL(Path);
    LogJSON(FString::Printf(TEXT("REQUEST %s %s"), *Method, *Path), Body.IsEmpty() ? TEXT("<empty>") : Body);
    TSharedRef<IHttpRequest, ESPMode::ThreadSafe> Request = FHttpModule::Get().CreateRequest();
    Request->SetURL(URL);
    Request->SetVerb(Method);
    Request->SetHeader(TEXT("Content-Type"), TEXT("application/json"));
    if (!DaemonToken.IsEmpty()) Request->SetHeader(TEXT("Authorization"), TEXT("Bearer ") + DaemonToken);
    if (!Body.IsEmpty()) Request->SetContentAsString(Body);
    Request->OnProcessRequestComplete().BindLambda([this, Method, Path, URL, Body, Callback](FHttpRequestPtr Req, FHttpResponsePtr Resp, bool bTransportOK)
    {
        const FString Text = Resp.IsValid() ? Resp->GetContentAsString() : TEXT("<no response>");
        const int32 Code = Resp.IsValid() ? Resp->GetResponseCode() : 0;
        const bool bSuccess = bTransportOK && Resp.IsValid() && Code >= 200 && Code < 300;
        FString Full = FString::Printf(TEXT("Method: %s\nPath: %s\nURL: %s\nHTTPStatus: %d\nResponseBody:\n%s"), *Method, *Path, *URL, Code, *Text);
        LogJSON(FString::Printf(TEXT("RESPONSE %s HTTP %d"), *Path, Code), Full);
        Callback(bSuccess, bSuccess ? Text : Full);
    });
    if (!Request->ProcessRequest()) Callback(false, FString::Printf(TEXT("HTTP request start failed: %s"), *URL));
}

FString FGameDepotUEModule::PathsToJSONBody(const TArray<FString>& Paths, const FString& ExtraFields) const
{
    FString Body = TEXT("{\"paths\":[");
    for (int32 i = 0; i < Paths.Num(); ++i)
    {
        if (i > 0) Body += TEXT(",");
        Body += QuoteJSON(Paths[i]);
    }
    Body += TEXT("]");
    if (!ExtraFields.IsEmpty()) Body += TEXT(",") + ExtraFields;
    Body += TEXT("}");
    return Body;
}

void FGameDepotUEModule::StartTaskEndpoint(const FString& Path, const FString& Body, const FString& FriendlyName)
{
    FNotificationInfo Info(FText::FromString(FriendlyName + TEXT(" started...")));
    Info.bFireAndForget = false;
    Info.bUseThrobber = true;
    ActiveTaskNotification = FSlateNotificationManager::Get().AddNotification(Info);
    if (ActiveTaskNotification.IsValid()) ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Pending);
    RequestJSON(TEXT("POST"), Path, Body, [this, FriendlyName](bool bOK, const FString& Text)
    {
        FString TaskID;
        if (bOK && TryGetStringField(Text, TEXT("task_id"), TaskID)) { PollTask(TaskID, FriendlyName); return; }
        if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetText(FText::FromString(FriendlyName + TEXT(" failed to start"))); ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Fail); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
        SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(FriendlyName + TEXT(" failed to start."));
    });
}

void FGameDepotUEModule::PollTask(const FString& TaskID, const FString& FriendlyName)
{
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/tasks/") + TaskID, TEXT(""), [this, TaskID, FriendlyName](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(FriendlyName + TEXT(" task poll failed.")); return; }
        TSharedPtr<FJsonObject> TaskObj;
        if (!TryGetObjectField(Text, TEXT("task"), TaskObj) || !TaskObj.IsValid()) { SetToolbarState(EGameDepotToolbarState::Error, TEXT("Invalid task response.")); return; }
        FString Status, Phase, Message, Error;
        TaskObj->TryGetStringField(TEXT("status"), Status);
        TaskObj->TryGetStringField(TEXT("phase"), Phase);
        TaskObj->TryGetStringField(TEXT("message"), Message);
        TaskObj->TryGetStringField(TEXT("error"), Error);
        if (ActiveTaskNotification.IsValid()) ActiveTaskNotification.Pin()->SetText(FText::FromString(FriendlyName + TEXT(": ") + Phase + TEXT(" ") + Message));
        if (Status == TEXT("succeeded"))
        {
            if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Success); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
            SetToolbarState(EGameDepotToolbarState::OK, FriendlyName + TEXT(" completed.")); Notify(ToolbarStateMessage); RefreshData();
        }
        else if (Status == TEXT("failed"))
        {
            if (ActiveTaskNotification.IsValid()) { ActiveTaskNotification.Pin()->SetCompletionState(SNotificationItem::CS_Fail); ActiveTaskNotification.Pin()->ExpireAndFadeout(); }
            SetToolbarState(EGameDepotToolbarState::Error, Error.IsEmpty() ? Text : Error); Notify(FriendlyName + TEXT(" failed."));
        }
        else
        {
            FTSTicker::GetCoreTicker().AddTicker(FTickerDelegate::CreateLambda([this, TaskID, FriendlyName](float) { PollTask(TaskID, FriendlyName); return false; }), 0.5f);
        }
    });
}

void FGameDepotUEModule::PullConfigFromDaemon(bool bRefreshPanel)
{
    if (bMockMode || !EnsureDaemon()) return;
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/config"), TEXT(""), [this, bRefreshPanel](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); return; }
        TSharedPtr<FJsonObject> Root;
        if (!ParseJsonObject(Text, Root)) return;
        FGameDepotConfigSnapshot Snap = ConfigManager.IsValid() ? ConfigManager->GetSnapshot() : FGameDepotConfigSnapshot();
        Snap.bInitialized = true;
        const TSharedPtr<FJsonObject>* OSS = nullptr; if (Root->TryGetObjectField(TEXT("oss"), OSS) && OSS && OSS->IsValid()) { (*OSS)->TryGetStringField(TEXT("provider"), Snap.OSSProvider); (*OSS)->TryGetStringField(TEXT("endpoint"), Snap.OSSEndpoint); (*OSS)->TryGetStringField(TEXT("bucket"), Snap.OSSBucket); (*OSS)->TryGetStringField(TEXT("region"), Snap.OSSRegion); (*OSS)->TryGetStringField(TEXT("blob_prefix"), Snap.OSSPrefix); }
        if (ConfigManager.IsValid()) ConfigManager->SetSnapshot(Snap);
        RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/rules"), TEXT(""), [this, bRefreshPanel, Snap](bool bRulesOK, const FString& RulesText) mutable
        {
            if (bRulesOK)
            {
                TSharedPtr<FJsonObject> RulesRoot;
                if (ParseJsonObject(RulesText, RulesRoot))
                {
                    const TArray<TSharedPtr<FJsonValue>>* Rules = nullptr;
                    if (RulesRoot->TryGetArrayField(TEXT("rules"), Rules) && Rules)
                    {
                        Snap.Rules.Reset();
                        for (const TSharedPtr<FJsonValue>& V : *Rules)
                        {
                            const TSharedPtr<FJsonObject> R = V.IsValid() ? V->AsObject() : nullptr;
                            if (!R.IsValid()) continue;
                            FGameDepotRuleConfig Rule;
                            R->TryGetStringField(TEXT("pattern"), Rule.Pattern);
                            R->TryGetStringField(TEXT("mode"), Rule.Mode);
                            R->TryGetStringField(TEXT("scope"), Rule.Scope);
                            if (Rule.Scope.IsEmpty()) Rule.Scope = TEXT("glob");
                            if (!Rule.Pattern.IsEmpty()) Snap.Rules.Add(Rule);
                        }
                        Snap.RuleText = FGameDepotConfigManager::RulesToText(Snap.Rules);
                        if (ConfigManager.IsValid()) ConfigManager->SetSnapshot(Snap);
                    }
                }
            }
            if (bRefreshPanel) { if (TSharedPtr<SGameDepotConfigPanel> Panel = ConfigPanel.Pin()) Panel->RefreshFromManager(); }
        });
    });
}

void FGameDepotUEModule::PushConfigToDaemon(const FGameDepotConfigSnapshot& Snapshot)
{
    if (!EnsureDaemon()) return;
    const FString Body = FString::Printf(TEXT("{\"oss\":{\"provider\":%s,\"endpoint\":%s,\"bucket\":%s,\"region\":%s,\"blob_prefix\":%s}}"), *QuoteJSON(Snapshot.OSSProvider), *QuoteJSON(Snapshot.OSSEndpoint), *QuoteJSON(Snapshot.OSSBucket), *QuoteJSON(Snapshot.OSSRegion), *QuoteJSON(Snapshot.OSSPrefix));
    RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/config"), Body, [this, Snapshot](bool bOK, const FString& Text)
    {
        if (!bOK) { SetToolbarState(EGameDepotToolbarState::Error, Text); Notify(TEXT("Save config failed.")); return; }
        for (const FGameDepotRuleConfig& Rule : Snapshot.Rules)
        {
            if (Rule.Pattern.TrimStartAndEnd().IsEmpty()) continue;
            const FString RuleBody = FString::Printf(TEXT("{\"pattern\":%s,\"mode\":%s,\"scope\":%s}"), *QuoteJSON(Rule.Pattern), *QuoteJSON(Rule.Mode), *QuoteJSON(Rule.Scope.IsEmpty() ? TEXT("glob") : Rule.Scope));
            RequestJSON(TEXT("POST"), TEXT("/api/ue/v1/rules/upsert"), RuleBody, [](bool, const FString&) {});
        }
        SetToolbarState(EGameDepotToolbarState::PendingUpdate, TEXT("Config saved to daemon. Sync or submit is pending."));
        Notify(TEXT("GameDepot config saved."));
        PullConfigFromDaemon(true);
        RefreshData();
    });
}

void FGameDepotUEModule::RefreshOverview()
{
    if (bMockMode || !EnsureDaemon()) return;
    RequestJSON(TEXT("GET"), TEXT("/api/ue/v1/overview"), TEXT(""), [this](bool bOK, const FString& Text)
    {
        if (bOK) ParseAndApplyOverview(Text);
    });
}

void FGameDepotUEModule::ParseAndApplyOverview(const FString& JsonText)
{
    TSharedPtr<FJsonObject> Root;
    if (!ParseJsonObject(JsonText, Root)) return;
    const TSharedPtr<FJsonObject>* Changes = nullptr;
    int32 Modified = 0, NewFiles = 0, Deleted = 0, LocalOnly = 0;
    if (Root->TryGetObjectField(TEXT("changes"), Changes) && Changes && Changes->IsValid())
    {
        double N = 0;
        if ((*Changes)->TryGetNumberField(TEXT("modified"), N)) Modified = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("new_files"), N)) NewFiles = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("deleted"), N)) Deleted = static_cast<int32>(N);
        if ((*Changes)->TryGetNumberField(TEXT("local_only"), N)) LocalOnly = static_cast<int32>(N);
    }
    const int32 Pending = Modified + NewFiles + Deleted + LocalOnly;
    if (Pending > 0) SetToolbarState(EGameDepotToolbarState::PendingUpdate, FString::Printf(TEXT("Pending update: %d modified, %d new/local, %d deleted."), Modified, NewFiles + LocalOnly, Deleted));
    else SetToolbarState(EGameDepotToolbarState::OK, TEXT("Workspace is clean."));
}

#undef LOCTEXT_NAMESPACE
IMPLEMENT_MODULE(FGameDepotUEModule, GameDepotUE)
