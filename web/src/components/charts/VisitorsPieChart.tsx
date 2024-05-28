import React from 'react';
import { BaseCard } from '@app/components/common/BaseCard/BaseCard';
import { useTranslation } from 'react-i18next';
import { PieChart } from '../common/charts/PieChart';
import useChartData from '@app/hooks/useChartData';  // Import the custom hook

export const VisitorsPieChart: React.FC = () => {
  const { t } = useTranslation();
  const { chartData, isLoading } = useChartData(); // Use the custom hook to get data

  const name = t('charts.visitorsFrom');

  return (
    <BaseCard padding="0 0 1.875rem" title={t('charts.pie')}>
        {isLoading || !chartData
            ? <p>{t('common.loading')}</p>
            : <PieChart data={chartData} name={name} showLegend={true} />}
    </BaseCard>
);

};

export default VisitorsPieChart;





