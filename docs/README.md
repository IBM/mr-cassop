# Cassandra Operator Documentation

## Running Locally

To download and run this locally, clone the repo and then go to the docs directory:

```console
cd ./cassandra-operator/docs
```

If `npm` is not already installed it can be installed with:

```console
brew install npm
```

Then install the dependencies:

```console
npm install
```

Now, there are actually 2 ways to built and serve the documentation locally (use either the first _OR_ the second option):

1. If you just want to view/read documentation:
   ```console
   npm run serve
   ```

1. If you develop the documentation and want to watch the changes:
   ```console
   npm run dev
   ```
