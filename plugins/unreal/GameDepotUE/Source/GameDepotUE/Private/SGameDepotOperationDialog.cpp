#include "SGameDepotOperationDialog.h"

#include "Styling/AppStyle.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/Layout/SScrollBox.h"
#include "Widgets/Notifications/SProgressBar.h"
#include "Widgets/SBoxPanel.h"
#include "Widgets/Text/STextBlock.h"

#define LOCTEXT_NAMESPACE "SGameDepotOperationDialog"

void SGameDepotOperationDialog::Construct(const FArguments& InArgs)
{
    OperationName = InArgs._OperationName;
    LogText.Reset();

    ChildSlot
    [
        SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
        .Padding(14.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)
            [
                SAssignNew(TitleText, STextBlock)
                .Text(FText::FromString(OperationName))
                .Font(FAppStyle::GetFontStyle("HeadingMedium"))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 6.0f)
            [
                SAssignNew(PhaseText, STextBlock)
                .Text(LOCTEXT("Preparing", "Preparing..."))
                .Font(FAppStyle::GetFontStyle("NormalFontBold"))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 8.0f)
            [
                SAssignNew(CurrentText, STextBlock)
                .Text(FText::GetEmpty())
                .AutoWrapText(true)
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 0.0f, 0.0f, 10.0f)
            [
                SAssignNew(ProgressBar, SProgressBar)
                .Percent(0.0f)
            ]
            + SVerticalBox::Slot().FillHeight(1.0f)
            [
                SNew(SBorder)
                .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
                .Padding(8.0f)
                [
                    SNew(SScrollBox)
                    + SScrollBox::Slot()
                    [
                        SAssignNew(LogTextBlock, STextBlock)
                        .Text(FText::GetEmpty())
                        .AutoWrapText(true)
                        .Font(FAppStyle::GetFontStyle("SmallFont"))
                    ]
                ]
            ]
        ]
    ];
}

void SGameDepotOperationDialog::SetProgress(const FString& Phase, const FString& Message, int32 Current, int32 Total, int32 Percent)
{
    if (PhaseText.IsValid())
    {
        PhaseText->SetText(FText::FromString(Phase.IsEmpty() ? TEXT("Running") : Phase));
    }
    if (CurrentText.IsValid())
    {
        FString Text = Message;
        if (Total > 0)
        {
            Text += FString::Printf(TEXT("  (%d / %d)"), Current, Total);
        }
        CurrentText->SetText(FText::FromString(Text));
    }
    if (ProgressBar.IsValid())
    {
        const float P = Total > 0 ? FMath::Clamp(static_cast<float>(Current) / static_cast<float>(Total), 0.0f, 1.0f) : FMath::Clamp(static_cast<float>(Percent) / 100.0f, 0.0f, 1.0f);
        ProgressBar->SetPercent(P);
    }
}

void SGameDepotOperationDialog::SetSucceeded(const FString& Message)
{
    SetProgress(TEXT("Complete"), Message, 1, 1, 100);
    AppendLog(Message);
}

void SGameDepotOperationDialog::SetFailed(const FString& Message)
{
    SetProgress(TEXT("Failed"), Message, 1, 1, 100);
    AppendLog(TEXT("error: ") + Message);
}

void SGameDepotOperationDialog::AppendLog(const FString& Line)
{
    if (Line.IsEmpty())
    {
        return;
    }
    if (!LogText.IsEmpty())
    {
        LogText += TEXT("\n");
    }
    LogText += Line;
    if (LogTextBlock.IsValid())
    {
        LogTextBlock->SetText(FText::FromString(LogText));
    }
}

#undef LOCTEXT_NAMESPACE

void SGameDepotOperationDialog::SetLogs(const TArray<FString>& Lines)
{
    LogText.Reset();
    for (int32 i = 0; i < Lines.Num(); ++i)
    {
        if (i > 0)
        {
            LogText += TEXT("\n");
        }
        LogText += Lines[i];
    }
    if (LogTextBlock.IsValid())
    {
        LogTextBlock->SetText(FText::FromString(LogText));
    }
}
