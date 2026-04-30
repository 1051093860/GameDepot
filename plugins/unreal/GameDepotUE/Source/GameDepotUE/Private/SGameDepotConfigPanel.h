#pragma once

#include "CoreMinimal.h"
#include "Widgets/SCompoundWidget.h"
#include "GameDepotConfigManager.h"

class SEditableTextBox;
template<typename ItemType> class SComboBox;
class SWidgetSwitcher;
class SVerticalBox;

class SGameDepotConfigPanel : public SCompoundWidget
{
public:
    SLATE_BEGIN_ARGS(SGameDepotConfigPanel) {}
        SLATE_ARGUMENT(TSharedPtr<FGameDepotConfigManager>, ConfigManager)
        SLATE_EVENT(FSimpleDelegate, OnConfigSaved)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs);
    void RefreshFromManager();

private:
    enum class EConfigPage : uint8
    {
        General = 0,
        Rules = 1
    };

    TSharedPtr<FGameDepotConfigManager> ConfigManager;
    FSimpleDelegate OnConfigSaved;

    EConfigPage ActivePage = EConfigPage::General;
    TSharedPtr<SWidgetSwitcher> PageSwitcher;
    TSharedPtr<SVerticalBox> RuleListBox;

    TSharedPtr<SEditableTextBox> GitRemoteBox;
    TSharedPtr<SEditableTextBox> GitBranchBox;
    TSharedPtr<SEditableTextBox> OSSProviderBox;
    TSharedPtr<SEditableTextBox> OSSEndpointBox;
    TSharedPtr<SEditableTextBox> OSSBucketBox;
    TSharedPtr<SEditableTextBox> OSSRegionBox;
    TSharedPtr<SEditableTextBox> OSSPrefixBox;

    TArray<TSharedPtr<FGameDepotRuleConfig>> RuleRows;
    TArray<TSharedPtr<FString>> ModeOptions;
    FText LastTestResult;

    TSharedRef<SWidget> BuildHeader();
    TSharedRef<SWidget> BuildNavigation();
    TSharedRef<SWidget> BuildNavButton(const FText& Label, EConfigPage Page, const FName& IconName);
    TSharedRef<SWidget> BuildPages();
    TSharedRef<SWidget> BuildGeneralPage();
    TSharedRef<SWidget> BuildRulesPage();
    TSharedRef<SWidget> BuildOSSSection();
    TSharedRef<SWidget> BuildRuleSection();
    TSharedRef<SWidget> BuildActions();
    TSharedRef<SWidget> BuildField(const FText& Label, const FText& Hint, TSharedPtr<SEditableTextBox>& OutBox, const FString& Value, bool bPassword = false);
    TSharedRef<SWidget> BuildRuleRowHeader();
    TSharedRef<SWidget> BuildRuleRow(TSharedPtr<FGameDepotRuleConfig> RuleItem, int32 Index);
    TSharedRef<SWidget> BuildModeCombo(TSharedPtr<FGameDepotRuleConfig> RuleItem);
    TSharedPtr<FString> FindModeOption(const FString& Mode) const;
    FText GetModeDisplayText(const FString& Mode) const;

    void SetActivePage(EConfigPage Page);
    void RebuildRuleList();
    void LoadRuleRowsFromSnapshot(const FGameDepotConfigSnapshot& Snapshot);
    TArray<FGameDepotRuleConfig> RulesFromRows() const;

    FGameDepotConfigSnapshot SnapshotFromFields() const;
    FReply OnInitializeClicked();
    FReply OnSaveClicked();
    FReply OnReloadClicked();
    FReply OnTestGitClicked();
    FReply OnTestOSSClicked();
    FReply OnTestRulesClicked();
    FReply OnAddRuleClicked();

    FText GetInitText() const;
    FText GetPathText() const;
    FText GetValidationText() const;
    FText GetLastTestText() const;
};
