import { useState, useEffect } from 'react';

interface Transaction {
  id: number;
  address: string;
  date: string;
  output: string;
  value: number;
}

interface BalanceData {
  balance_usd: number;
}

const useBalanceData = () => {
  const [balanceData, setBalanceData] = useState<BalanceData | null>(null);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true);
      try {
        // Fetch balance data
        const balanceResponse = await fetch('http://localhost:5000/balance/usd');
        if (!balanceResponse.ok) {
          throw new Error(`Network response was not ok (status: ${balanceResponse.status})`);
        }
        const balanceData = await balanceResponse.json();
        setBalanceData(balanceData);

        // Fetch transaction data
        const transactionsResponse = await fetch('http://localhost:5000/transactions/latest');
        if (!transactionsResponse.ok) {
          throw new Error(`Network response was not ok (status: ${transactionsResponse.status})`);
        }
        const transactionsData = await transactionsResponse.json();
        setTransactions(transactionsData);
      } catch (error) {
        console.error('Error fetching data:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchData();
  }, []);

  return { balanceData, transactions, isLoading };
};

export default useBalanceData;
