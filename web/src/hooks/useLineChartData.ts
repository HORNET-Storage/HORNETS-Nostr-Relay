import { useState, useEffect } from 'react';

interface TimeSeriesData {
  month: string;
  profiles: number;
  lightning_addr: number;
  dht_key: number;
  lightning_and_dht: number;
}

const useLineChartData = () => {
  const [data, setData] = useState<TimeSeriesData[] | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        const response = await fetch('http://localhost:5000/timeseries', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({}), // If needed, include a payload here
        });
        if (!response.ok) {
          throw new Error(`Network response was not ok (status: ${response.status})`);
        }
        const data = await response.json();
        console.log('Data:', data);
        setData(data);
      } catch (error) {
        console.error('Error:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchData();
  }, []);

  return { data, isLoading };
};

export default useLineChartData;

