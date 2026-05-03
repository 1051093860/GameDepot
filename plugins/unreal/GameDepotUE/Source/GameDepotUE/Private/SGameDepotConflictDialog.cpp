#include "SGameDepotConflictDialog.h"

#include "Styling/AppStyle.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/Layout/SScrollBox.h"
#include "Widgets/SBoxPanel.h"
#include "Widgets/Text/STextBlock.h"
#include "Widgets/Views/SHeaderRow.h"

#define LOCTEXT_NAMESPACE "SGameDepotConflictDialog"

class SGameDepotConflictRow : public SMultiColumnTableRow<FGameDepotConflictItemPtr>
{
public:
    SLATE_BEGIN_ARGS(SGameDepotConflictRow) {}
        SLATE_ARGUMENT(FGameDepotConflictItemPtr, Item)
        SLATE_EVENT(FGameDepotResolveConflictAction, OnResolveRequested)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs, const TSharedRef<STableViewBase>& OwnerTable)
    {
        Item = InArgs._Item;
        OnResolveRequested = InArgs._OnResolveRequested;
        SMultiColumnTableRow<FGameDepotConflictItemPtr>::Construct(FSuperRowType::FArguments().Padding(FMargin(4.0f, 6.0f)), OwnerTable);
    }

    virtual TSharedRef<SWidget> GenerateWidgetForColumn(const FName& ColumnName) override
    {
        if (!Item.IsValid())
        {
            return SNew(STextBlock).Text(FText::GetEmpty());
        }
        if (ColumnName == FName(TEXT("Asset")))
        {
            return SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight()
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Item->Path))
                    .Font(FAppStyle::GetFontStyle("NormalFontBold"))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 3.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Item->Kind))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.72f, 0.72f, 0.72f)))
                    .Font(FAppStyle::GetFontStyle("SmallFont"))
                ];
        }
        if (ColumnName == FName(TEXT("Versions")))
        {
            return SNew(STextBlock)
                .Text(FText::FromString(FString::Printf(TEXT("base=%s\nlocal=%s\nremote=%s"), *Item->BaseOID, *Item->LocalOID, *Item->RemoteOID)))
                .Font(FAppStyle::GetFontStyle("SmallFont"));
        }
        if (ColumnName == FName(TEXT("Actions")))
        {
            return SNew(SHorizontalBox)
                + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
                [
                    SNew(SButton)
                    .Text(LOCTEXT("UseRemote", "Use Remote"))
                    .ToolTipText(LOCTEXT("UseRemoteTip", "Replace the local Content file with the remote version."))
                    .OnClicked_Lambda([this]()
                    {
                        if (Item.IsValid() && OnResolveRequested.IsBound())
                        {
                            OnResolveRequested.Execute(Item->Path, TEXT("remote"));
                        }
                        return FReply::Handled();
                    })
                ]
                + SHorizontalBox::Slot().AutoWidth()
                [
                    SNew(SButton)
                    .Text(LOCTEXT("KeepLocal", "Keep Local and Publish"))
                    .ToolTipText(LOCTEXT("KeepLocalTip", "Keep the local file, upload it, create a new commit, and push it to remote."))
                    .OnClicked_Lambda([this]()
                    {
                        if (Item.IsValid() && OnResolveRequested.IsBound())
                        {
                            OnResolveRequested.Execute(Item->Path, TEXT("local"));
                        }
                        return FReply::Handled();
                    })
                ];
        }
        return SNew(STextBlock).Text(FText::GetEmpty());
    }

private:
    FGameDepotConflictItemPtr Item;
    FGameDepotResolveConflictAction OnResolveRequested;
};

void SGameDepotConflictDialog::Construct(const FArguments& InArgs)
{
    Conflicts = InArgs._Conflicts;
    OnResolveRequested = InArgs._OnResolveRequested;

    ChildSlot
    [
        SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(14.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)
            [
                SNew(STextBlock)
                .Text(LOCTEXT("Title", "GameDepot Conflicts"))
                .Font(FAppStyle::GetFontStyle("HeadingMedium"))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 10.0f)
            [
                SNew(STextBlock)
                .Text(LOCTEXT("Subtitle", "Choose one version for each conflicting Content asset. Keeping local will immediately publish it back to the remote branch."))
                .AutoWrapText(true)
            ]
            + SVerticalBox::Slot().FillHeight(1.0f)
            [
                SAssignNew(ListView, SListView<FGameDepotConflictItemPtr>)
                .ListItemsSource(&Conflicts)
                .OnGenerateRow(this, &SGameDepotConflictDialog::MakeRow)
                .HeaderRow
                (
                    SNew(SHeaderRow)
                    + SHeaderRow::Column(TEXT("Asset")).DefaultLabel(LOCTEXT("AssetHeader", "Asset")).FillWidth(0.45f)
                    + SHeaderRow::Column(TEXT("Versions")).DefaultLabel(LOCTEXT("VersionsHeader", "Versions")).FillWidth(0.25f)
                    + SHeaderRow::Column(TEXT("Actions")).DefaultLabel(LOCTEXT("ActionsHeader", "Actions")).FillWidth(0.30f)
                )
            ]
        ]
    ];
}

TSharedRef<ITableRow> SGameDepotConflictDialog::MakeRow(FGameDepotConflictItemPtr Item, const TSharedRef<STableViewBase>& OwnerTable)
{
    return SNew(SGameDepotConflictRow, OwnerTable)
        .Item(Item)
        .OnResolveRequested(OnResolveRequested);
}

FText SGameDepotConflictDialog::ExplainKind(const FString& Kind) const
{
    if (Kind.Contains(TEXT("deleted")))
    {
        return LOCTEXT("DeleteConflict", "Delete/modify conflict");
    }
    return LOCTEXT("ModifyConflict", "Both local and remote changed this asset");
}

#undef LOCTEXT_NAMESPACE
