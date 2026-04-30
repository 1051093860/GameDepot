#include "SGameDepotConfigPanel.h"

#include "Framework/Notifications/NotificationManager.h"
#include "Styling/AppStyle.h"
#include "Widgets/Images/SImage.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/Input/SComboBox.h"
#include "Widgets/Input/SEditableTextBox.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/Layout/SBox.h"
#include "Widgets/Layout/SScrollBox.h"
#include "Widgets/Layout/SSeparator.h"
#include "Widgets/Layout/SWidgetSwitcher.h"
#include "Widgets/Notifications/SNotificationList.h"
#include "Widgets/Text/STextBlock.h"

#define LOCTEXT_NAMESPACE "SGameDepotConfigPanel"

namespace
{
void NotifyConfigPanel(const FString& Message)
{
    FNotificationInfo Info(FText::FromString(Message));
    Info.ExpireDuration = 3.0f;
    Info.bUseLargeFont = false;
    FSlateNotificationManager::Get().AddNotification(Info);
}

TSharedRef<SWidget> SectionTitle(const FText& Text)
{
    return SNew(STextBlock)
        .Text(Text)
        .Font(FAppStyle::GetFontStyle("HeadingSmall"))
        .ColorAndOpacity(FSlateColor(FLinearColor(0.90f, 0.90f, 0.90f)));
}

FSlateColor PageButtonColor(bool bSelected)
{
    return bSelected
        ? FSlateColor(FLinearColor(0.20f, 0.35f, 0.75f, 1.0f))
        : FSlateColor(FLinearColor(0.10f, 0.10f, 0.10f, 0.35f));
}
}

void SGameDepotConfigPanel::Construct(const FArguments& InArgs)
{
    ConfigManager = InArgs._ConfigManager;
    OnConfigSaved = InArgs._OnConfigSaved;
    LastTestResult = LOCTEXT("NoTestYet", "No test has been run yet.");
    ModeOptions.Reset();
    ModeOptions.Add(MakeShared<FString>(TEXT("git")));
    ModeOptions.Add(MakeShared<FString>(TEXT("blob")));
    ModeOptions.Add(MakeShared<FString>(TEXT("ignore")));

    if (ConfigManager.IsValid())
    {
        ConfigManager->Load();
        LoadRuleRowsFromSnapshot(ConfigManager->GetSnapshot());
    }

    ChildSlot
    [
        SNew(SVerticalBox)
        + SVerticalBox::Slot().AutoHeight().Padding(10.0f, 10.0f, 10.0f, 8.0f)
        [
            BuildHeader()
        ]
        + SVerticalBox::Slot().FillHeight(1.0f).Padding(10.0f, 0.0f, 10.0f, 8.0f)
        [
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 10.0f, 0.0f)
            [
                BuildNavigation()
            ]
            + SHorizontalBox::Slot().FillWidth(1.0f)
            [
                BuildPages()
            ]
        ]
        + SVerticalBox::Slot().AutoHeight().Padding(10.0f, 0.0f, 10.0f, 10.0f)
        [
            BuildActions()
        ]
    ];

    RefreshFromManager();
}

void SGameDepotConfigPanel::RefreshFromManager()
{
    if (!ConfigManager.IsValid())
    {
        return;
    }

    const FGameDepotConfigSnapshot& Snapshot = ConfigManager->GetSnapshot();
    if (OSSProviderBox.IsValid()) OSSProviderBox->SetText(FText::FromString(Snapshot.OSSProvider));
    if (OSSEndpointBox.IsValid()) OSSEndpointBox->SetText(FText::FromString(Snapshot.OSSEndpoint));
    if (OSSBucketBox.IsValid()) OSSBucketBox->SetText(FText::FromString(Snapshot.OSSBucket));
    if (OSSRegionBox.IsValid()) OSSRegionBox->SetText(FText::FromString(Snapshot.OSSRegion));
    if (OSSPrefixBox.IsValid()) OSSPrefixBox->SetText(FText::FromString(Snapshot.OSSPrefix));

    LoadRuleRowsFromSnapshot(Snapshot);
    RebuildRuleList();
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildHeader()
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(10.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight()
            [
                SNew(STextBlock)
                .Text(LOCTEXT("Title", "GameDepot Configuration Manager"))
                .Font(FAppStyle::GetFontStyle("HeadingMedium"))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 6.0f, 0.0f, 0.0f)
            [
                SNew(STextBlock)
                .Text(this, &SGameDepotConfigPanel::GetInitText)
                .ColorAndOpacity(FSlateColor(FLinearColor(0.82f, 0.82f, 0.82f)))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 4.0f, 0.0f, 0.0f)
            [
                SNew(STextBlock)
                .Text(this, &SGameDepotConfigPanel::GetPathText)
                .ColorAndOpacity(FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f)))
                .AutoWrapText(true)
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildNavigation()
{
    return SNew(SBox)
        .WidthOverride(148.0f)
        [
            SNew(SBorder)
            .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
            .Padding(8.0f)
            [
                SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 6.0f)
                [
                    SNew(STextBlock)
                    .Text(LOCTEXT("ConfigSections", "Sections"))
                    .Font(FAppStyle::GetFontStyle("SmallFontBold"))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.60f, 0.60f, 0.60f)))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 4.0f)
                [
                    BuildNavButton(LOCTEXT("GeneralNav", "General"), EConfigPage::General, "Icons.Settings")
                ]
                + SVerticalBox::Slot().AutoHeight()
                [
                    BuildNavButton(LOCTEXT("RulesNav", "Rules"), EConfigPage::Rules, "Icons.Filter")
                ]
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildNavButton(const FText& Label, EConfigPage Page, const FName& IconName)
{
    return SNew(SButton)
        .ButtonColorAndOpacity_Lambda([this, Page]() { return PageButtonColor(ActivePage == Page); })
        .OnClicked_Lambda([this, Page]()
        {
            SetActivePage(Page);
            return FReply::Handled();
        })
        [
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().AutoWidth().VAlign(VAlign_Center).Padding(0.0f, 0.0f, 6.0f, 0.0f)
            [
                SNew(SImage)
                .Image(FAppStyle::GetBrush(IconName))
            ]
            + SHorizontalBox::Slot().FillWidth(1.0f).VAlign(VAlign_Center)
            [
                SNew(STextBlock).Text(Label)
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildPages()
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(10.0f)
        [
            SAssignNew(PageSwitcher, SWidgetSwitcher)
            + SWidgetSwitcher::Slot()
            [
                BuildGeneralPage()
            ]
            + SWidgetSwitcher::Slot()
            [
                BuildRulesPage()
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildGeneralPage()
{
    return SNew(SScrollBox)
        + SScrollBox::Slot().Padding(0.0f, 0.0f, 0.0f, 8.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight()
            [
                BuildOSSSection()
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildRulesPage()
{
    return SNew(SScrollBox)
        + SScrollBox::Slot()
        [
            BuildRuleSection()
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildField(const FText& Label, const FText& Hint, TSharedPtr<SEditableTextBox>& OutBox, const FString& Value, bool bPassword)
{
    return SNew(SHorizontalBox)
        + SHorizontalBox::Slot().VAlign(VAlign_Center).AutoWidth().Padding(0.0f, 0.0f, 8.0f, 4.0f)
        [
            SNew(STextBlock)
            .Text(Label)
            .MinDesiredWidth(120.0f)
        ]
        + SHorizontalBox::Slot().FillWidth(1.0f).Padding(0.0f, 0.0f, 0.0f, 4.0f)
        [
            SAssignNew(OutBox, SEditableTextBox)
            .Text(FText::FromString(Value))
            .HintText(Hint)
            .IsPassword(bPassword)
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildOSSSection()
{
    const FGameDepotConfigSnapshot Snapshot = ConfigManager.IsValid() ? ConfigManager->GetSnapshot() : FGameDepotConfigSnapshot();
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
        .Padding(10.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)[SectionTitle(LOCTEXT("OSSTitle", "OSS / Object Storage"))]
            + SVerticalBox::Slot().AutoHeight()[BuildField(LOCTEXT("OSSProvider", "Provider"), LOCTEXT("OSSProviderHint", "aliyun-oss / cos / s3"), OSSProviderBox, Snapshot.OSSProvider)]
            + SVerticalBox::Slot().AutoHeight()[BuildField(LOCTEXT("OSSEndpoint", "Endpoint"), LOCTEXT("OSSEndpointHint", "oss-cn-hangzhou.aliyuncs.com"), OSSEndpointBox, Snapshot.OSSEndpoint)]
            + SVerticalBox::Slot().AutoHeight()[BuildField(LOCTEXT("OSSBucket", "Bucket"), LOCTEXT("OSSBucketHint", "your-game-bucket"), OSSBucketBox, Snapshot.OSSBucket)]
            + SVerticalBox::Slot().AutoHeight()[BuildField(LOCTEXT("OSSRegion", "Region"), LOCTEXT("OSSRegionHint", "cn-hangzhou"), OSSRegionBox, Snapshot.OSSRegion)]
            + SVerticalBox::Slot().AutoHeight()[BuildField(LOCTEXT("OSSPrefix", "Blob Prefix"), LOCTEXT("OSSPrefixHint", "gamedepot/blobs"), OSSPrefixBox, Snapshot.OSSPrefix)]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 6.0f, 0.0f, 0.0f)
            [
                SNew(SButton)
                .Text(LOCTEXT("TestOSS", "Test OSS Config"))
                .OnClicked(this, &SGameDepotConfigPanel::OnTestOSSClicked)
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildRuleSection()
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
        .Padding(10.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)
            [
                SectionTitle(LOCTEXT("RulesTitle", "Rules"))
            ]
            + SVerticalBox::Slot().AutoHeight()
            [
                SNew(STextBlock)
                .Text(LOCTEXT("RulesHelp", "Editable rule list. Pattern decides matching, Mode decides Git or OSS/Blob storage. Kind is hidden for now."))
                .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 8.0f, 0.0f, 4.0f)
            [
                BuildRuleRowHeader()
            ]
            + SVerticalBox::Slot().AutoHeight()
            [
                SAssignNew(RuleListBox, SVerticalBox)
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 8.0f, 0.0f, 0.0f)
            [
                SNew(SHorizontalBox)
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
                [
                    SNew(SButton)
                    .Text(LOCTEXT("AddRule", "Add Rule"))
                    .OnClicked(this, &SGameDepotConfigPanel::OnAddRuleClicked)
                ]
                + SHorizontalBox::Slot().AutoWidth()
                [
                    SNew(SButton)
                    .Text(LOCTEXT("TestRules", "Test Rules"))
                    .OnClicked(this, &SGameDepotConfigPanel::OnTestRulesClicked)
                ]
            ]
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildRuleRowHeader()
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Header"))
        .Padding(6.0f)
        [
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().FillWidth(3.2f)[SNew(STextBlock).Text(LOCTEXT("PatternHeader", "Pattern"))]
            + SHorizontalBox::Slot().FillWidth(1.0f).Padding(6.0f, 0.0f)[SNew(STextBlock).Text(LOCTEXT("ModeHeader", "Mode"))]
            + SHorizontalBox::Slot().FillWidth(0.9f).Padding(6.0f, 0.0f)[SNew(STextBlock).Text(LOCTEXT("ScopeHeader", "Scope"))]
            + SHorizontalBox::Slot().AutoWidth()[SNew(STextBlock).Text(LOCTEXT("ActionsHeader", "Actions"))]
        ];
}

FText SGameDepotConfigPanel::GetModeDisplayText(const FString& Mode) const
{
    const FString LowerMode = Mode.ToLower();
    if (LowerMode == TEXT("git"))
    {
        return LOCTEXT("ModeGitDisplay", "Git");
    }
    if (LowerMode == TEXT("blob"))
    {
        return LOCTEXT("ModeBlobDisplay", "OSS Blob");
    }
    if (LowerMode == TEXT("ignore"))
    {
        return LOCTEXT("ModeIgnoreDisplay", "Ignore");
    }
    if (LowerMode == TEXT("review"))
    {
        return LOCTEXT("ModeReviewDisplay", "Review");
    }
    return FText::FromString(Mode);
}

TSharedPtr<FString> SGameDepotConfigPanel::FindModeOption(const FString& Mode) const
{
    const FString LowerMode = Mode.ToLower();
    for (const TSharedPtr<FString>& Option : ModeOptions)
    {
        if (Option.IsValid() && Option->Equals(LowerMode, ESearchCase::IgnoreCase))
        {
            return Option;
        }
    }
    return ModeOptions.Num() > 0 ? ModeOptions[0] : TSharedPtr<FString>();
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildModeCombo(TSharedPtr<FGameDepotRuleConfig> RuleItem)
{
    TSharedPtr<FString> InitialSelection = FindModeOption(RuleItem.IsValid() ? RuleItem->Mode : TEXT("git"));

    return SNew(SComboBox<TSharedPtr<FString>>)
        .OptionsSource(&ModeOptions)
        .InitiallySelectedItem(InitialSelection)
        .OnGenerateWidget_Lambda([this](TSharedPtr<FString> Item)
        {
            return SNew(STextBlock)
                .Text(Item.IsValid() ? GetModeDisplayText(*Item) : FText::GetEmpty());
        })
        .OnSelectionChanged_Lambda([RuleItem](TSharedPtr<FString> NewSelection, ESelectInfo::Type)
        {
            if (RuleItem.IsValid() && NewSelection.IsValid())
            {
                RuleItem->Mode = NewSelection->ToLower();
            }
        })
        [
            SNew(STextBlock)
            .Text_Lambda([this, RuleItem]()
            {
                return RuleItem.IsValid() ? GetModeDisplayText(RuleItem->Mode) : FText::GetEmpty();
            })
        ];
}

TSharedRef<SWidget> SGameDepotConfigPanel::BuildRuleRow(TSharedPtr<FGameDepotRuleConfig> RuleItem, int32 Index)
{
    auto IconButton = [](const FName& IconName, const FText& Tooltip, bool bEnabled, TFunction<FReply()> Handler)
    {
        return SNew(SButton)
            .ButtonStyle(FAppStyle::Get(), TEXT("SimpleButton"))
            .ToolTipText(Tooltip)
            .ContentPadding(FMargin(3.0f))
            .IsEnabled(bEnabled)
            .OnClicked_Lambda([Handler]() { return Handler(); })
            [
                SNew(SImage)
                .Image(FAppStyle::GetBrush(IconName))
            ];
    };

    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(4.0f)
        [
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().FillWidth(3.2f).VAlign(VAlign_Center)
            [
                SNew(SEditableTextBox)
                .Text_Lambda([RuleItem]() { return RuleItem.IsValid() ? FText::FromString(RuleItem->Pattern) : FText::GetEmpty(); })
                .HintText(LOCTEXT("PatternHint", "Content/**/*.uasset"))
                .OnTextChanged_Lambda([RuleItem](const FText& NewText) { if (RuleItem.IsValid()) RuleItem->Pattern = NewText.ToString(); })
            ]
            + SHorizontalBox::Slot().FillWidth(1.0f).Padding(6.0f, 0.0f).VAlign(VAlign_Center)
            [
                BuildModeCombo(RuleItem)
            ]
            + SHorizontalBox::Slot().FillWidth(0.9f).Padding(6.0f, 0.0f).VAlign(VAlign_Center)
            [
                SNew(SEditableTextBox)
                .Text_Lambda([RuleItem]() { return RuleItem.IsValid() ? FText::FromString(RuleItem->Scope) : FText::GetEmpty(); })
                .HintText(LOCTEXT("ScopeHint", "glob"))
                .OnTextChanged_Lambda([RuleItem](const FText& NewText) { if (RuleItem.IsValid()) RuleItem->Scope = NewText.ToString().ToLower(); })
            ]
            + SHorizontalBox::Slot().AutoWidth().VAlign(VAlign_Center)
            [
                SNew(SHorizontalBox)
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 3.0f, 0.0f)
                [
                    IconButton(FName(TEXT("Icons.ArrowUp")), LOCTEXT("RuleUpTip", "Move rule up"), Index > 0, [this, Index]()
                    {
                        if (RuleRows.IsValidIndex(Index) && RuleRows.IsValidIndex(Index - 1))
                        {
                            RuleRows.Swap(Index, Index - 1);
                            RebuildRuleList();
                        }
                        return FReply::Handled();
                    })
                ]
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 3.0f, 0.0f)
                [
                    IconButton(FName(TEXT("Icons.ArrowDown")), LOCTEXT("RuleDownTip", "Move rule down"), Index < RuleRows.Num() - 1, [this, Index]()
                    {
                        if (RuleRows.IsValidIndex(Index) && RuleRows.IsValidIndex(Index + 1))
                        {
                            RuleRows.Swap(Index, Index + 1);
                            RebuildRuleList();
                        }
                        return FReply::Handled();
                    })
                ]
                + SHorizontalBox::Slot().AutoWidth()
                [
                    IconButton(FName(TEXT("Icons.Delete")), LOCTEXT("RuleDeleteTip", "Delete rule"), true, [this, Index]()
                    {
                        if (RuleRows.IsValidIndex(Index))
                        {
                            RuleRows.RemoveAt(Index);
                            RebuildRuleList();
                        }
                        return FReply::Handled();
                    })
                ]
            ]
        ];
}


TSharedRef<SWidget> SGameDepotConfigPanel::BuildActions()
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(10.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight()
            [
                SNew(SHorizontalBox)
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
                [
                    SNew(SButton)
                    .Text(LOCTEXT("Initialize", "Initialize Workspace"))
                    .OnClicked(this, &SGameDepotConfigPanel::OnInitializeClicked)
                ]
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
                [
                    SNew(SButton)
                    .Text(LOCTEXT("Save", "Save Config"))
                    .OnClicked(this, &SGameDepotConfigPanel::OnSaveClicked)
                ]
                + SHorizontalBox::Slot().AutoWidth()
                [
                    SNew(SButton)
                    .Text(LOCTEXT("Reload", "Reload"))
                    .OnClicked(this, &SGameDepotConfigPanel::OnReloadClicked)
                ]
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 8.0f, 0.0f, 0.0f)
            [
                SNew(STextBlock)
                .Text(this, &SGameDepotConfigPanel::GetValidationText)
                .AutoWrapText(true)
                .ColorAndOpacity(FSlateColor(FLinearColor(0.82f, 0.82f, 0.82f)))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 6.0f, 0.0f, 0.0f)
            [
                SNew(STextBlock)
                .Text(this, &SGameDepotConfigPanel::GetLastTestText)
                .AutoWrapText(true)
                .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.78f, 1.00f)))
            ]
        ];
}

void SGameDepotConfigPanel::SetActivePage(EConfigPage Page)
{
    ActivePage = Page;
    if (PageSwitcher.IsValid())
    {
        PageSwitcher->SetActiveWidgetIndex(Page == EConfigPage::General ? 0 : 1);
    }
}

void SGameDepotConfigPanel::LoadRuleRowsFromSnapshot(const FGameDepotConfigSnapshot& Snapshot)
{
    RuleRows.Reset();
    for (const FGameDepotRuleConfig& Rule : Snapshot.Rules)
    {
        RuleRows.Add(MakeShared<FGameDepotRuleConfig>(Rule));
    }
}

TArray<FGameDepotRuleConfig> SGameDepotConfigPanel::RulesFromRows() const
{
    TArray<FGameDepotRuleConfig> Rules;
    for (const TSharedPtr<FGameDepotRuleConfig>& RuleItem : RuleRows)
    {
        if (RuleItem.IsValid())
        {
            Rules.Add(*RuleItem);
        }
    }
    return Rules;
}

void SGameDepotConfigPanel::RebuildRuleList()
{
    if (!RuleListBox.IsValid())
    {
        return;
    }

    RuleListBox->ClearChildren();
    for (int32 Index = 0; Index < RuleRows.Num(); ++Index)
    {
        RuleListBox->AddSlot()
        .AutoHeight()
        .Padding(0.0f, 0.0f, 0.0f, 4.0f)
        [
            BuildRuleRow(RuleRows[Index], Index)
        ];
    }

    if (RuleRows.Num() == 0)
    {
        RuleListBox->AddSlot()
        .AutoHeight()
        .Padding(0.0f, 4.0f)
        [
            SNew(STextBlock)
            .Text(LOCTEXT("NoRules", "No rules yet. Click Add Rule to create one."))
            .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)))
        ];
    }
}

FGameDepotConfigSnapshot SGameDepotConfigPanel::SnapshotFromFields() const
{
    FGameDepotConfigSnapshot Snapshot = ConfigManager.IsValid() ? ConfigManager->GetSnapshot() : FGameDepotConfigSnapshot();
    Snapshot.OSSProvider = OSSProviderBox.IsValid() ? OSSProviderBox->GetText().ToString() : Snapshot.OSSProvider;
    Snapshot.OSSEndpoint = OSSEndpointBox.IsValid() ? OSSEndpointBox->GetText().ToString() : Snapshot.OSSEndpoint;
    Snapshot.OSSBucket = OSSBucketBox.IsValid() ? OSSBucketBox->GetText().ToString() : Snapshot.OSSBucket;
    Snapshot.OSSRegion = OSSRegionBox.IsValid() ? OSSRegionBox->GetText().ToString() : Snapshot.OSSRegion;
    Snapshot.OSSPrefix = OSSPrefixBox.IsValid() ? OSSPrefixBox->GetText().ToString() : Snapshot.OSSPrefix;
    Snapshot.Rules = RulesFromRows();
    Snapshot.RuleText = FGameDepotConfigManager::RulesToText(Snapshot.Rules);
    return Snapshot;
}

FReply SGameDepotConfigPanel::OnInitializeClicked()
{
    if (!ConfigManager.IsValid())
    {
        return FReply::Handled();
    }
    FString Error;
    if (ConfigManager->InitializeDefault(Error))
    {
        LastTestResult = LOCTEXT("Initialized", "Workspace initialized with default Git / OSS / Rule config.");
        RefreshFromManager();
        if (OnConfigSaved.IsBound())
        {
            OnConfigSaved.Execute();
        }
        NotifyConfigPanel(TEXT("GameDepot workspace initialized."));
    }
    else
    {
        LastTestResult = FText::FromString(Error);
        NotifyConfigPanel(Error);
    }
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnSaveClicked()
{
    if (!ConfigManager.IsValid())
    {
        return FReply::Handled();
    }
    FString Error;
    if (ConfigManager->Save(SnapshotFromFields(), Error))
    {
        LastTestResult = LOCTEXT("Saved", "Config saved. Toolbar state should become pending update until Sync / Submit is run.");
        if (OnConfigSaved.IsBound())
        {
            OnConfigSaved.Execute();
        }
        NotifyConfigPanel(TEXT("GameDepot mock config saved."));
    }
    else
    {
        LastTestResult = FText::FromString(Error);
        NotifyConfigPanel(Error);
    }
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnReloadClicked()
{
    if (ConfigManager.IsValid()) ConfigManager->Load();
    RefreshFromManager();
    LastTestResult = LOCTEXT("Reloaded", "Config reloaded from disk.");
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnTestGitClicked()
{
    LastTestResult = LOCTEXT("GitNative", "Git remotes are configured with native git commands, not GameDepot.");
    NotifyConfigPanel(LastTestResult.ToString());
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnTestOSSClicked()
{
    const FGameDepotConfigSnapshot Snapshot = SnapshotFromFields();
    if (Snapshot.OSSProvider.IsEmpty() || Snapshot.OSSBucket.IsEmpty() || Snapshot.OSSRegion.IsEmpty())
    {
        LastTestResult = LOCTEXT("OSSTestBad", "OSS config check failed: provider, bucket, and region are required.");
    }
    else
    {
        LastTestResult = FText::FromString(FString::Printf(TEXT("OSS config check OK. Provider=%s Bucket=%s Region=%s"), *Snapshot.OSSProvider, *Snapshot.OSSBucket, *Snapshot.OSSRegion));
    }
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnTestRulesClicked()
{
    const TArray<FGameDepotRuleConfig> Rules = RulesFromRows();
    if (Rules.Num() == 0)
    {
        LastTestResult = LOCTEXT("RuleTestBadEmpty", "Rule check failed: no rules configured.");
        return FReply::Handled();
    }

    int32 BlobCount = 0;
    int32 GitCount = 0;
    int32 IgnoreCount = 0;
    int32 BadCount = 0;
    for (const FGameDepotRuleConfig& Rule : Rules)
    {
        const FString Mode = Rule.Mode.ToLower();
        if (Rule.Pattern.TrimStartAndEnd().IsEmpty())
        {
            ++BadCount;
        }
        else if (Mode == TEXT("blob"))
        {
            ++BlobCount;
        }
        else if (Mode == TEXT("git"))
        {
            ++GitCount;
        }
        else if (Mode == TEXT("ignore"))
        {
            ++IgnoreCount;
        }
        else
        {
            ++BadCount;
        }
    }

    if (BadCount > 0)
    {
        LastTestResult = FText::FromString(FString::Printf(TEXT("Rule check found %d invalid rule(s). Check empty patterns or unsupported modes."), BadCount));
    }
    else
    {
        LastTestResult = FText::FromString(FString::Printf(TEXT("Rule check OK. %d rules: %d OSS/blob, %d Git, %d Ignore."), Rules.Num(), BlobCount, GitCount, IgnoreCount));
    }
    return FReply::Handled();
}

FReply SGameDepotConfigPanel::OnAddRuleClicked()
{
    FGameDepotRuleConfig Rule;
    Rule.Pattern = TEXT("Content/**");
    Rule.Mode = TEXT("blob");
    Rule.Kind = TEXT("");
    Rule.Scope = TEXT("glob");
    RuleRows.Add(MakeShared<FGameDepotRuleConfig>(Rule));
    RebuildRuleList();
    LastTestResult = LOCTEXT("RuleAdded", "Added a new editable rule row.");
    return FReply::Handled();
}

FText SGameDepotConfigPanel::GetInitText() const
{
    if (!ConfigManager.IsValid())
    {
        return LOCTEXT("NoManager", "Config manager is unavailable.");
    }
    return ConfigManager->IsInitialized()
        ? LOCTEXT("InitializedText", "Initialized: yes")
        : LOCTEXT("NotInitializedText", "Initialized: no. Use Initialize Workspace or Save Config.");
}


FText SGameDepotConfigPanel::GetPathText() const
{
    if (!ConfigManager.IsValid())
    {
        return FText::GetEmpty();
    }
    return FText::FromString(FString::Printf(TEXT("Config: %s\nRules:  %s"), *ConfigManager->GetConfigPath(), *ConfigManager->GetRulesPath()));
}

FText SGameDepotConfigPanel::GetValidationText() const
{
    if (!ConfigManager.IsValid())
    {
        return LOCTEXT("NoValidation", "No config manager.");
    }

    const FGameDepotConfigSnapshot Snapshot = SnapshotFromFields();
    TArray<FString> Issues;
    if (!ConfigManager->IsInitialized())
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
        return LOCTEXT("ConfigLooksValid", "Config looks valid for the daemon UI. Real connectivity checks will be attached to the daemon later.");
    }
    return FText::FromString(FString::Join(Issues, TEXT("\n")));
}

FText SGameDepotConfigPanel::GetLastTestText() const
{
    return LastTestResult;
}

#undef LOCTEXT_NAMESPACE
