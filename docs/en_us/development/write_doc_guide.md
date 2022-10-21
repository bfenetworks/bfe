# How to contribute documentation

BFE encourages you to contribute documentation. If your written or translated documents meet our requirements, your documents will be available on the bfe-networks.com website and on Github for BFE users.

BFE's documentation is mainly divided into the following categories:

- Beginner's Guide: to help users get started and inspired;

- User Guides: to provide users with a tutorial of basic operations in BFE;

- Developer Guides: to meet the needs of developers;

Our documentation supports contributions in format of [Markdown](https://guides.github.com/features/ Mastering-markdown/) (GitHub style) .

Once the document is written, you can use the preview tool to check how the document appears to verify that your document is displayed correctly on the official website.

## How to use the preview tool

### Install its dependencies

Before doing this, please make sure your operating system has gitbook installed.

Take the ubuntu system as an example, run:

```bash
$ sudo apt-get update && apt-get install -y npm
$ sudo npm install -g gitbook-cli
```

### Clone related repository:

First download the full repository:

```bash
$ git clone https://github.com/bfenetworks/bfe
```

### Run document site locally

Change to base directory of documents which you want to load and build(docs/LANG), run:

```bash
$ cd docs/en_us/
$ gitbook serve --port 8000
...
Serving book on http://localhost:8000
```

Then: open your browser and navigate to http://localhost:8000.

>* The site may take a few seconds to load because the building takes a certain amount of time*

## Contriubute documents

All content should be written in [Markdown](https://guides.github.com/features/mastering-markdown/) (GitHub style).

### Contribute new documents

- Create a new `.md` file or modify an existing article in the repository you are currently working on
- Add the new document name to the corresponding index file (SUMMARY.md)

### Run the preview tool

- Run the preview tool in base directory of documents (docs/LANG)

```bash
$ cd docs/en_us/
$ gitbook serve --port 8000
```

### Preview modification

Open your browser and navigate to http://localhost:8000 .

On the page to be updated, click Refresh Content at the top right corner.

## Pull Request for your changes

The steps to submit changes and PR can refer to [How to contribute code](../development/local_dev_guide.md)

## Help improve preview tool

We encourage your contributions to all aspects of the platform and supportive contents. You can Fork or Clone repository, ask questions and feedback, or submit bugs on issues. For details, please refer to the [Development Guide](https://github.com/bfenetworks/bfe/blob/develop/README.md).
