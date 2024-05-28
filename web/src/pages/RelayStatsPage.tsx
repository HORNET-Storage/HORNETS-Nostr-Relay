import React from 'react';
import { useTranslation } from 'react-i18next';
import { VisitorsPieChart } from '@app/components/charts/VisitorsPieChart';
import { LineRaceChart } from '@app/components/charts/LineRaceChart/LineRaceChart';
import { PageTitle } from '@app/components/common/PageTitle/PageTitle';
import { BaseRow } from '@app/components/common/BaseRow/BaseRow';
import { BaseCol } from '@app/components/common/BaseCol/BaseCol';

const RelayStatsPage: React.FC = () => {
  const { t } = useTranslation();
  const shouldDisplayPieChart = true; // This could be dynamically determined
  return (
    <>
      <PageTitle>{t('relay.stats')}</PageTitle>
      <BaseRow gutter={[30, 30]}>
        <BaseCol id="line-race" xs={24} lg={12}>
          <LineRaceChart />
        </BaseCol>
        {shouldDisplayPieChart && (
          <BaseCol id="pie" xs={24} lg={12}>
            <VisitorsPieChart />
          </BaseCol>
        )}
      </BaseRow>
    </>
  );
};

export default RelayStatsPage;
